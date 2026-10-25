package app

import (
	"context"
	"fmt"

	"github.com/mojocn/base64Captcha"

	"myblogx/middleware"
	"myblogx/router"
)

// WireHTTP 集中组装 HTTP handler 与 router。
//
// 依赖只拼一份（middleware.RuntimeOptions），各模块的处理器都在这里注入 router。
// 旧的 api.Api 容器已随 captcha 迁入 platform 而删除 —— 路由文件不再接收容器参数。
func WireHTTP(infra *Infra, lc *Lifecycle) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	cfg := infra.Config
	// 验证码的生成（internal/platform/captcha）与校验（middleware）必须是同一个 store，
	// 否则前端拿到的 captcha_id 在登录时校验不到。这里只创建一个实例，两处共用。
	captchaStore := base64Captcha.DefaultMemStore
	opts := middleware.RuntimeOptions{
		System:            cfg.System,
		JWT:               cfg.Jwt,
		Log:               cfg.Log,
		ClickHouseConfig:  cfg.ClickHouse,
		Logger:            infra.Logger,
		DB:                infra.DB,
		Redis:             infra.Redis,
		ClickHouse:        infra.ClickHouse,
		RuntimeSite:       infra.RuntimeSite,
		ImageCaptchaStore: captchaStore,
		// 认证只有一份实现：中间件与路由都走 internal/auth。
		Verifier: authVerifier{},
	}

	// captcha 链路：图片验证码改由 internal/platform/captcha 提供。
	WireCaptcha(infra, captchaStore)

	// media 链路：图片能力改由 internal/media + platform/storage 提供。
	WireMedia(infra)

	// site 链路：站点配置与轮播图改由 internal/site 提供。
	WireSite(infra)

	// article 链路：启动时注入处理器。旧实现已删除，这是唯一路径。
	WireArticle(infra)

	// user + auth 链路：认证路由改由 internal/auth 提供。
	WireUser(infra)

	// comment 链路：路由改由 internal/comment 提供。
	WireComment(infra)

	// notification 链路：站内信与全局公告改由 internal/notification 提供。
	// 必须早于 outbox：中继的订阅者要靠它发通知。
	WireNotification(infra)

	// search 链路：路由改由 internal/search 提供。
	WireSearch(infra)

	// social 链路：关注关系改由 internal/social 提供。
	WireSocial(infra)

	// chat 链路：私信改由 internal/chat 提供。
	WireChat(infra)

	// ai 链路：AI 能力改由 internal/ai 提供。
	WireAI(infra)

	// analytics 链路：站点统计与日志查询改由 internal/analytics 提供。
	// 必须在 WireUser 之后：用户列表的"最后登录"依赖同一份日志查询配置。
	WireAnalytics(infra)

	srv, err := router.Run(opts)
	if err != nil {
		return fmt.Errorf("启动 HTTP 服务失败: %w", err)
	}

	// 把 HTTP 服务注册为停机钩子：收到 SIGTERM 后先停止接收新请求，
	// 再在 stopTimeout 内等待在途请求结束（这是原先完全缺失的一环）。
	if lc != nil {
		lc.OnStop("http-server", func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		})
	}

	return nil
}
