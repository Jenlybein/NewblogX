package app

import (
	"context"
	"fmt"

	"myblogx/api"
	"myblogx/internal/platform/mysql"
	"myblogx/middleware"
	"myblogx/router"

	"github.com/mojocn/base64Captcha"
)

// WireHTTP 集中组装 HTTP handler 与 router。
//
// 依赖只拼一份（middleware.Deps），同时交给 router 与 api.New 使用，
// 避免过去 apideps.Deps + api.Deps 两份字段完全重复的字面量。
func WireHTTP(infra *Infra, lc *Lifecycle) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	cfg := infra.Config
	deps := middleware.Deps{
		Version:           infra.Version,
		ConfigFile:        infra.ConfigFile,
		System:            cfg.System,
		JWT:               cfg.Jwt,
		Log:               cfg.Log,
		ClickHouseConfig:  cfg.ClickHouse,
		ES:                cfg.ES,
		QQ:                cfg.QQ,
		Email:             cfg.Email,
		QiNiu:             cfg.QiNiu,
		Upload:            cfg.Upload,
		Logger:            infra.Logger,
		DB:                infra.DB,
		Redis:             infra.Redis,
		ClickHouse:        infra.ClickHouse,
		ESClient:          infra.ESClient,
		RuntimeSite:       infra.RuntimeSite,
		ImageCaptchaStore: base64Captcha.DefaultMemStore,
		Tx:                mysql.NewTxManager(infra.DB),
	}

	// media 链路：图片能力改由 internal/media + platform/storage 提供。
	WireMedia(infra)

	// article 链路：启动时注入处理器。旧实现已删除，这是唯一路径。
	WireArticle(infra)

	// user + auth 链路：认证路由改由 internal/auth 提供。
	WireUser(infra)

	// comment 链路：路由改由 internal/comment 提供。
	WireComment(infra)

	// search 链路：路由改由 internal/search 提供。
	WireSearch(infra)

	// social 链路：关注关系改由 internal/social 提供。
	WireSocial(infra)

	// chat 链路：私信改由 internal/chat 提供。
	WireChat(infra)

	srv, err := router.Run(deps, api.New(deps))
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
