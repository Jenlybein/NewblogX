package image_service

import (
	"encoding/json"
	"errors"
	"fmt"
	"myblogx/conf"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

type qiniuRuntime struct {
	mac        *auth.Credentials
	bucketMgr  *storage.BucketManager
	httpClient *http.Client
}

var (
	qiniuRuntimeMu   sync.Mutex
	qiniuRuntimeKey  string
	qiniuRuntimeInst *qiniuRuntime
)

func getQiniuRuntime(q conf.QiNiu) *qiniuRuntime {
	qiniuRuntimeMu.Lock()
	defer qiniuRuntimeMu.Unlock()

	confKey := fmt.Sprintf("%s|%s", q.AccessKey, q.SecretKey)
	if qiniuRuntimeInst == nil || qiniuRuntimeKey != confKey {
		mac := auth.New(q.AccessKey, q.SecretKey)
		cfg := &storage.Config{UseHTTPS: true}
		qiniuRuntimeInst = &qiniuRuntime{
			mac:       mac,
			bucketMgr: storage.NewBucketManager(mac, cfg),
			httpClient: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
		qiniuRuntimeKey = confKey
	}
	return qiniuRuntimeInst
}

func VerifyQiniuCallback(deps Deps, req *http.Request) (bool, error) {
	return getQiniuRuntime(deps.QiNiu).mac.VerifyCallback(req)
}

func StatObject(deps Deps, bucket, key string) (*storage.FileInfo, error) {
	info, err := getQiniuRuntime(deps.QiNiu).bucketMgr.Stat(bucket, key)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func DeleteObject(deps Deps, bucket, key string) error {
	return getQiniuRuntime(deps.QiNiu).bucketMgr.Delete(bucket, key)
}

func ImageInfoObject(deps Deps, bucket, key string) (*ImageInfoResult, error) {
	_ = bucket

	q := deps.QiNiu
	if strings.TrimSpace(q.Uri) == "" {
		return nil, errors.New("七牛下载域名未配置")
	}

	domain := strings.TrimRight(strings.TrimSpace(q.Uri), "/")
	deadline := time.Now().Add(3 * time.Minute).Unix()
	downloadURL := storage.MakePrivateURLv2(getQiniuRuntime(q).mac, domain, key, deadline)
	if strings.Contains(downloadURL, "?") {
		downloadURL += "&imageInfo"
	} else {
		downloadURL += "?imageInfo"
	}

	resp, err := getQiniuRuntime(q).httpClient.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("七牛图片信息读取失败，状态码 %d", resp.StatusCode)
	}

	var result ImageInfoResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Format == "" || result.Width <= 0 || result.Height <= 0 {
		return nil, errors.New("七牛图片信息不完整")
	}
	return &result, nil
}

func ObjectURL(deps Deps, key string) string {
	domain := strings.TrimRight(strings.TrimSpace(deps.QiNiu.Uri), "/")
	if domain == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return storage.MakePublicURLv2(domain, key)
}
