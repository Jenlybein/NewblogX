// Package qiniu 把平台七牛客户端适配成媒体模块的对象存储端口。
//
// 单独成包而不是写在装配层：媒体装配与 CLI 维护命令都要用同一条适配，
// 复制一份的结果是"修了一处、另一处还是旧行为"。
package qiniu

import (
	"net/http"

	"myblogx/conf"
	"myblogx/internal/media/domain"
	platformqiniu "myblogx/internal/platform/storage/qiniu"
)

// Storage 实现 domain.ObjectStorage。
type Storage struct {
	// cfg 是函数：七牛的密钥/桶名/域名都是运行时配置，每次现读，
	// 后台改完下一次调用立即生效，不必重建进程内的客户端。
	cfg func() conf.QiNiu
}

var _ domain.ObjectStorage = (*Storage)(nil)

// New 构造适配器。cfg 为空时按空配置（调用方拿不到有效域名/桶，会自然失败）。
func New(cfg func() conf.QiNiu) *Storage {
	if cfg == nil {
		cfg = func() conf.QiNiu { return conf.QiNiu{} }
	}
	return &Storage{cfg: cfg}
}

// inner 按当前配置构造平台客户端（连接池由平台包按配置指纹缓存）。
func (s *Storage) inner() *platformqiniu.Storage {
	return platformqiniu.New(s.cfg())
}

// Token 签发上传凭证。
func (s *Storage) Token(p domain.Policy) (domain.Token, error) {
	got, err := s.inner().Token(platformqiniu.Policy{
		Bucket:      p.Bucket,
		ObjectKey:   p.ObjectKey,
		CallbackURL: p.CallbackURL,
		ExpireAt:    p.ExpireAt,
		MaxSize:     p.MaxSize,
		EndUser:     p.EndUser,
	})
	if err != nil {
		return domain.Token{}, err
	}
	return domain.Token{Token: got.Token, Bucket: got.Bucket, ObjectKey: got.ObjectKey, ExpireAt: got.ExpireAt}, nil
}

// VerifyCallback 校验回调签名。
func (s *Storage) VerifyCallback(req *http.Request) (bool, error) {
	return s.inner().VerifyCallback(req)
}

// Stat 读对象元信息。
func (s *Storage) Stat(bucket, key string) (domain.ObjectInfo, error) {
	got, err := s.inner().Stat(bucket, key)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	return domain.ObjectInfo{Hash: got.Hash, Size: got.Size}, nil
}

// Delete 删除对象。
func (s *Storage) Delete(bucket, key string) error { return s.inner().Delete(bucket, key) }

// ImageInfo 读图片尺寸。
func (s *Storage) ImageInfo(key string) (domain.ImageInfo, error) {
	got, err := s.inner().ImageInfo(key)
	if err != nil {
		return domain.ImageInfo{}, err
	}
	return domain.ImageInfo{Format: got.Format, Width: got.Width, Height: got.Height, Size: got.Size}, nil
}

// ObjectURL 拼接访问地址。
func (s *Storage) ObjectURL(key string) string { return s.inner().ObjectURL(key) }
