// Package qiniu 封装七牛对象存储：上传凭证、回调校验、对象元信息、删除。
//
// 从 service/image_service 迁出（S4.3）：业务模块只依赖 media 的端口，
// 真正的 SDK 调用收在这一层，换存储商时不用动业务代码。
package qiniu

import (
	"context"
	"errors"
	"fmt"
	"myblogx/internal/apperr"
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

	// censorEndpoint 留空时用 censorEndpoint 常量；测试里指向本地服务器，
	// 这样能只验证签名而不碰真七牛。
	censorEndpoint string
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
		return Token{}, apperr.New(apperr.CodeStorageBucketRequired)
	}
	if p.ObjectKey == "" {
		return Token{}, apperr.New(apperr.CodeStorageObjectKeyRequired)
	}
	if p.MaxSize <= 0 {
		return Token{}, apperr.New(apperr.CodeStorageSizeInvalid)
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
// Delete 删除对象。
//
// 语义是**幂等**的：目标状态是"对象不存在"，所以"本来就不存在"也算成功。
//
// 不吞掉 612（no such file）会留下一条永远删不掉的记录：图片被判违规时
// `audit_auto_delete_block` 已经删过一次对象，运营再点「删除」就每次都失败 ——
// 2026-11-25 生产上的表现是后台点删除"没反应"（前端吞了 code 非 0 的响应），
// 而 DB 里那条 block 行再也清不掉，CDN 上其实早就没有这个文件了。
func (s *Storage) Delete(bucket, key string) error {
	err := getRuntime(s.cfg).bucketMgr.Delete(bucket, key)
	if isNoSuchFile(err) {
		return nil
	}
	return err
}

// isNoSuchFile 判断七牛错误是不是"东西已经不存在了"。
//
// 612 = no such file or directory；631 = no such bucket（桶都没了，对象自然也没了）。
// 两个都按"目标状态已达成"处理。SDK 有些路径只回字符串，所以留一条文案兜底。
func isNoSuchFile(err error) bool {
	if err == nil {
		return false
	}
	var info *storage.ErrorInfo
	if errors.As(err, &info) {
		return info.Code == 612 || info.Code == 631
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such file")
}

// mediaBase 返回规范化后的资源访问域名：始终带 https:// 协议头。
//
// 站点在 HTTPS 下时，浏览器会把页面里 http 的图片"自动升级"成 https 再加载
// （Chrome / Firefox / Safari 都这样，且没有开关），所以发出去的图片地址只能是
// https。配置里写裸域名、写 http:// 或写 //，统一收敛成 https://。
func (s *Storage) mediaBase() string {
	domain := strings.TrimRight(strings.TrimSpace(s.cfg.Uri), "/")
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "https://") {
		return domain
	}
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "//")
	return "https://" + domain
}

// ObjectURL 拼接公开访问地址。
func (s *Storage) ObjectURL(key string) string {
	domain := s.mediaBase()
	if domain == "" {
		return ""
	}
	return storage.MakePublicURLv2(domain, key)
}

// PrivateURL 拼接带下载凭证的访问地址（私有空间）。
//
// 参数 key 是**已拼好样式后缀**的对象名，例如 `myblogx/images/20261102/hash-small`。
// 顺序不能反：七牛的下载凭证是对完整 URL 计算的，先对原图签名再往后追加 `-style`
// 会让令牌失效，表现为图片全部 401（官方文档「带图片样式的私有空间文件授权」）。
// SDK 内部会对 key 做 URL 编码，所以带后缀的 key 能正确参与签名。
func (s *Storage) PrivateURL(key string, expiry time.Duration) string {
	domain := s.mediaBase()
	if domain == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	if expiry <= 0 {
		expiry = defaultDownloadExpiry
	}
	deadline := time.Now().Add(expiry).Unix()
	return storage.MakePrivateURLv2(getRuntime(s.cfg).mac, domain, key, deadline)
}

// defaultDownloadExpiry 是"没配有效期"时的回落值（一小时）。
//
// 与上传凭证的 expiry 无关：那个只有几十秒到几分钟，拿来给图片链接签名会让
// 页面上的图很快全部 401。
const defaultDownloadExpiry = time.Hour

// ProbeResult 是一次探活的结果。
type ProbeResult struct {
	Status int
	Size   int64
	// Error 非空表示这次探测本身失败（网络错误等），此时 Status 为 0。
	Error string
}

// Probe 对一条完整 URL 发一次 HEAD，拿回状态码与字节数。
//
// 用途是后台的「样式探活」：样式名填错时七牛返回 **404 而不是回退原图**，
// 没有这一步只能等线上图片全裂才发现。
func (s *Storage) Probe(ctx context.Context, rawURL string) ProbeResult {
	if strings.TrimSpace(rawURL) == "" {
		return ProbeResult{Error: "地址为空"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return ProbeResult{Error: err.Error()}
	}
	resp, err := getRuntime(s.cfg).httpClient.Do(req)
	if err != nil {
		return ProbeResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	result := ProbeResult{Status: resp.StatusCode, Size: resp.ContentLength}
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		// 少数 CDN 不实现 HEAD：退回一次 GET，只要响应头，不读 body。
		getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return ProbeResult{Error: err.Error()}
		}
		getResp, err := getRuntime(s.cfg).httpClient.Do(getReq)
		if err != nil {
			return ProbeResult{Error: err.Error()}
		}
		defer getResp.Body.Close()
		result = ProbeResult{Status: getResp.StatusCode, Size: getResp.ContentLength}
	}
	return result
}
