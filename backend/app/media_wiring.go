package app

import (
	"net/http"

	mediahttp "myblogx/internal/media/adapter/http"
	mediamysql "myblogx/internal/media/adapter/mysql"
	mediaredis "myblogx/internal/media/adapter/redis"
	mediaapp "myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/storage/qiniu"
	"myblogx/router"
)

// WireMedia 装配媒体模块并注入路由处理器。
func WireMedia(infra *Infra) {
	if infra == nil {
		return
	}
	qiniuCfg := infra.Config.QiNiu
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)

	svc := mediaapp.New(
		mediamysql.New(infra.DB),
		mediaredis.NewTaskStore(redisOptions),
		mediaredis.NewAuditStore(redisOptions),
		&qiniuStorage{inner: qiniu.New(qiniuCfg)},
		mediaIDs{},
		realClock{},
		infra.Logger,
		domain.Config{
			Enabled:       qiniuCfg.Enable,
			Bucket:        qiniuCfg.Bucket,
			Prefix:        qiniuCfg.Prefix,
			Region:        qiniuCfg.Region,
			SizeMB:        qiniuCfg.Size,
			ExpirySeconds: qiniuCfg.Expiry,
			CallbackURL:   qiniuCfg.CallbackURL,
			Whitelist:     infra.Config.Upload.Whitelist,
		},
	)
	router.SetImageHandler(mediahttp.NewHandler(svc))
}

// mediaIDs 用雪花生成图片与任务 ID。
type mediaIDs struct{}

// NextImageID 返回下一个图片 ID。
func (mediaIDs) NextImageID() domain.ImageID {
	next, err := idgen.Next()
	if err != nil {
		panic("media: 雪花 ID 生成失败: " + err.Error())
	}
	return domain.ImageID(next.Uint64())
}

// NextTaskID 返回下一个上传任务 ID。
func (mediaIDs) NextTaskID() uint64 {
	next, err := idgen.Next()
	if err != nil {
		panic("media: 雪花 ID 生成失败: " + err.Error())
	}
	return next.Uint64()
}

// qiniuStorage 把 platform/storage/qiniu 适配成 media 的 ObjectStorage 端口。
//
// 平台包不依赖业务模块的 domain，转换放在装配层。
type qiniuStorage struct {
	inner *qiniu.Storage
}

var _ domain.ObjectStorage = (*qiniuStorage)(nil)

// Token 签发上传凭证。
func (s *qiniuStorage) Token(p domain.Policy) (domain.Token, error) {
	got, err := s.inner.Token(qiniu.Policy{
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
func (s *qiniuStorage) VerifyCallback(req *http.Request) (bool, error) {
	return s.inner.VerifyCallback(req)
}

// Stat 读对象元信息。
func (s *qiniuStorage) Stat(bucket, key string) (domain.ObjectInfo, error) {
	got, err := s.inner.Stat(bucket, key)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	return domain.ObjectInfo{Hash: got.Hash, Size: got.Size}, nil
}

// Delete 删除对象。
func (s *qiniuStorage) Delete(bucket, key string) error { return s.inner.Delete(bucket, key) }

// ImageInfo 读图片尺寸。
func (s *qiniuStorage) ImageInfo(key string) (domain.ImageInfo, error) {
	got, err := s.inner.ImageInfo(key)
	if err != nil {
		return domain.ImageInfo{}, err
	}
	return domain.ImageInfo{Format: got.Format, Width: got.Width, Height: got.Height, Size: got.Size}, nil
}

// ObjectURL 拼接访问地址。
func (s *qiniuStorage) ObjectURL(key string) string { return s.inner.ObjectURL(key) }
