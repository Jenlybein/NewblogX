// Package qiniu 封装七牛对象存储：上传凭证、回调校验、对象元信息、删除。
//
// 从 service/image_service 迁出（S4.3）：业务模块只依赖 media 的端口，
// 真正的 SDK 调用收在这一层，换存储商时不用动业务代码。
package qiniu

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"myblogx/conf"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

// Policy 是一次上传的约束。
type Policy struct {
	Bucket      string
	ObjectKey   string
	CallbackURL string
	ExpireAt    time.Time
	MaxSize     int64
	EndUser     string
}

// Token 是签发出来的上传凭证。
type Token struct {
	Token     string
	Bucket    string
	ObjectKey string
	ExpireAt  time.Time
}

// ObjectInfo 是对象存储上的文件元信息。
type ObjectInfo struct {
	Hash string
	Size int64
}

// ImageInfo 是图片的格式与尺寸。
type ImageInfo struct {
	Format string
	Width  int
	Height int
	Size   int64
}

// Storage 是七牛客户端。
type Storage struct {
	cfg conf.QiNiu
}

// New 构造七牛客户端。
func New(cfg conf.QiNiu) *Storage { return &Storage{cfg: cfg} }

type runtime struct {
	mac        *auth.Credentials
	bucketMgr  *storage.BucketManager
	httpClient *http.Client
}

var (
	runtimeMu   sync.Mutex
	runtimeKey  string
	runtimeInst *runtime
)

// getRuntime 复用 SDK 客户端（凭证变更时重建）。
func getRuntime(cfg conf.QiNiu) *runtime {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	key := fmt.Sprintf("%s|%s", cfg.AccessKey, cfg.SecretKey)
	if runtimeInst == nil || runtimeKey != key {
		mac := auth.New(cfg.AccessKey, cfg.SecretKey)
		runtimeInst = &runtime{
			mac:        mac,
			bucketMgr:  storage.NewBucketManager(mac, &storage.Config{UseHTTPS: true}),
			httpClient: &http.Client{Timeout: 10 * time.Second},
		}
		runtimeKey = key
	}
	return runtimeInst
}

// Token 生成上传凭证。
func (s *Storage) Token(p Policy) (Token, error) {
	if p.Bucket == "" {
		return Token{}, errors.New("qiniu: bucket 不能为空")
	}
	if p.ObjectKey == "" {
		return Token{}, errors.New("qiniu: object key 不能为空")
	}
	if p.MaxSize <= 0 {
		return Token{}, errors.New("qiniu: size limit 必须大于 0")
	}

	putPolicy := storage.PutPolicy{
		Scope:      fmt.Sprintf("%s:%s", p.Bucket, p.ObjectKey),
		Expires:    uint64(p.ExpireAt.Unix()),
		InsertOnly: 1,
		EndUser:    p.EndUser,
		FsizeLimit: p.MaxSize,
		DetectMime: 1,
		MimeLimit:  "image/*",
		ReturnBody: `{"key":"$(key)","hash":"$(etag)","bucket":"$(bucket)","fsize":$(fsize)}`,
	}
	if p.CallbackURL != "" {
		putPolicy.CallbackURL = p.CallbackURL
		putPolicy.CallbackBody = `{"key":"$(key)","hash":"$(etag)","bucket":"$(bucket)","fsize":$(fsize)}`
		putPolicy.CallbackBodyType = "application/json"
	}

	return Token{
		Token:     putPolicy.UploadToken(getRuntime(s.cfg).mac),
		Bucket:    p.Bucket,
		ObjectKey: p.ObjectKey,
		ExpireAt:  p.ExpireAt,
	}, nil
}

// VerifyCallback 校验七牛回调签名。
func (s *Storage) VerifyCallback(req *http.Request) (bool, error) {
	return getRuntime(s.cfg).mac.VerifyCallback(req)
}

// Stat 读取对象元信息。
func (s *Storage) Stat(bucket, key string) (ObjectInfo, error) {
	info, err := getRuntime(s.cfg).bucketMgr.Stat(bucket, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Hash: info.Hash, Size: info.Fsize}, nil
}

// Delete 删除对象。
func (s *Storage) Delete(bucket, key string) error {
	return getRuntime(s.cfg).bucketMgr.Delete(bucket, key)
}

// ImageInfo 读取图片尺寸（走七牛 imageInfo 接口）。
func (s *Storage) ImageInfo(key string) (ImageInfo, error) {
	domain := strings.TrimRight(strings.TrimSpace(s.cfg.Uri), "/")
	if domain == "" {
		return ImageInfo{}, errors.New("qiniu: 下载域名未配置")
	}

	deadline := time.Now().Add(3 * time.Minute).Unix()
	downloadURL := storage.MakePrivateURLv2(getRuntime(s.cfg).mac, domain, key, deadline)
	if strings.Contains(downloadURL, "?") {
		downloadURL += "&imageInfo"
	} else {
		downloadURL += "?imageInfo"
	}

	resp, err := getRuntime(s.cfg).httpClient.Get(downloadURL)
	if err != nil {
		return ImageInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ImageInfo{}, fmt.Errorf("qiniu: imageInfo 状态码 %d", resp.StatusCode)
	}

	var result ImageInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ImageInfo{}, err
	}
	if result.Format == "" || result.Width <= 0 || result.Height <= 0 {
		return ImageInfo{}, errors.New("qiniu: imageInfo 不完整")
	}
	return result, nil
}

// ObjectURL 拼接公开访问地址。
func (s *Storage) ObjectURL(key string) string {
	domain := strings.TrimRight(strings.TrimSpace(s.cfg.Uri), "/")
	if domain == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return storage.MakePublicURLv2(domain, key)
}
