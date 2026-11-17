package app

import (
	"context"
	"fmt"
	"time"

	"github.com/mojocn/base64Captcha"

	captchastore "myblogx/internal/platform/captcha"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
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
	//
	// 优先用 Redis：库自带的 DefaultMemStore 是进程内内存，(a) 重启即失效、
	// (b) 多副本时 A 生成 B 校验永远失败。两者在单实例下都不显形，专挑重启/扩容那天炸。
	// Redis 不可用（本地没起 Redis、单元测试）时退回内存存储，功能不退化、只是同进程内有效。
	var captchaStore base64Captcha.Store
	if infra.Redis != nil {
		captchaStore = captchastore.NewRedisStore(
			infra.Redis,
			// TTL 每次现读配置：改有效期不该要求重启，且只影响新生成的图片。
			func() time.Duration {
				return time.Duration(infra.RuntimeSite.GetRuntimeCaptcha().ExpireSeconds) * time.Second
			},
			infra.Logger,
		)
	} else {
		if infra.Logger != nil {
			infra.Logger.Warn("Redis 不可用，图形验证码退回进程内存储：重启即失效、多副本不通用")
		}
		captchaStore = base64Captcha.DefaultMemStore
	}
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

	// 内容过滤链路（票 396）：引擎已在 InitRuntimeServices 里按词表构建，
	// 这里只把「关键词风险」读面的处理器注入路由树。
	WireFilter(infra)

	// article 链路：启动时注入处理器。旧实现已删除，这是唯一路径。
	WireArticle(infra)

	// user + auth 链路：认证路由改由 internal/auth 提供。
	WireUser(infra)

	// comment 链路：路由改由 internal/comment 提供。
	WireComment(infra)

	// 关键词风险的处置写端口（票 398）：必须在文章 / 评论 / 用户三个模块装配
	// **之后** —— 处置要复用它们各自那一份用例服务，不能另建一份。
	WireFilterDisposer(infra)

	// notification 链路：站内信与全局公告改由 internal/notification 提供。
	// 必须早于 outbox：中继的订阅者要靠它发通知。
	WireNotification(infra)

	// search 链路：路由改由 internal/search 提供。
	WireSearch(infra)

	// social 链路：关注关系改由 internal/social 提供。
	WireSocial(infra)

	// chat 链路：私信改由 internal/chat 提供。
	// 处理器由模块装配后显式交给路由树（#280）：路由包不再持有私信包级变量。
	chatModule := WireChat(infra)
	handlers := router.Handlers{}
	if chatModule != nil {
		handlers.Chat = chatModule.Handler
	}

	// 发布闸门（票 399 / ADR-0032 决策 8）：被禁言者的发布类动作在各模块入口
	// 拒绝。必须排在 user / article / comment / chat 装配之后 —— 闸门接的是
	// 它们各自那一份服务实例。
	WirePublishGate(infra, chatModule)

	// ai 链路：AI 能力改由 internal/ai 提供。
	WireAI(infra)

	// analytics 链路：站点统计与日志查询改由 internal/analytics 提供。
	// 必须在 WireUser 之后：用户列表的"最后登录"依赖同一份日志查询配置。
	WireAnalytics(infra)

	srv, err := router.Run(opts, infra.Health, handlers)
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
