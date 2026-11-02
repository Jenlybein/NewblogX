package app

import (
	"myblogx/internal/kernel/ctype"

	mediahttp "myblogx/internal/media/adapter/http"
	mediamysql "myblogx/internal/media/adapter/mysql"
	mediaqiniu "myblogx/internal/media/adapter/qiniu"
	mediaredis "myblogx/internal/media/adapter/redis"
	mediaapp "myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
)

// WireMedia 装配媒体模块并注入路由处理器。
func WireMedia(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)

	svc := mediaapp.New(
		mediamysql.New(infra.DB),
		mediaredis.NewTaskStore(redisOptions),
		mediaredis.NewAuditStore(redisOptions),
		mediaqiniu.New(infra.RuntimeSite.GetRuntimeStorage),
		mediaIDs{},
		realClock{},
		infra.Logger,
		// 配置传函数而不是值：管理员在后台改完七牛密钥/桶名/白名单，
		// 下一次上传就要按新配置走，不能要求重启进程。
		func() domain.Config {
			qiniuCfg := infra.RuntimeSite.GetRuntimeStorage()
			return domain.Config{
				Enabled:       qiniuCfg.Enable,
				Bucket:        qiniuCfg.Bucket,
				Prefix:        qiniuCfg.Prefix,
				Region:        qiniuCfg.Region,
				SizeMB:        qiniuCfg.Size,
				ExpirySeconds: qiniuCfg.Expiry,
				CallbackURL:   qiniuCfg.CallbackURL,
				Whitelist:     infra.RuntimeSite.GetRuntimeUpload().Whitelist,
			}
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
	return ctype.NewID(next.Uint64())
}

// NextTaskID 返回下一个上传任务 ID。
func (mediaIDs) NextTaskID() uint64 {
	next, err := idgen.Next()
	if err != nil {
		panic("media: 雪花 ID 生成失败: " + err.Error())
	}
	return next.Uint64()
}
