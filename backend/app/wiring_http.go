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
