package middleware

import (
	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/redis"
	"myblogx/service/log_service"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
)

// Runtime 保存请求中间件所需依赖，由启动阶段一次性注入。
type Runtime struct {
	JWT               conf.Jwt
	LogConfig         conf.Logrus
	Logger            *logrus.Logger
	Redis             redis_service.Options
	Log               log_service.Options
	RuntimeSite       *sitecfg.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
	Authenticator     *user_service.Authenticator
}

func NewRuntime(opts RuntimeOptions) Runtime {
	redisOptions := redis_service.Options{
		Client: opts.Redis,
		Logger: opts.Logger,
	}
	return Runtime{
		JWT:       opts.JWT,
		LogConfig: opts.Log,
		Logger:    opts.Logger,
		Redis:     redisOptions,
		Log: log_service.Options{
			LogConfig:        opts.Log,
			SystemConfig:     opts.System,
			ClickHouseEnable: opts.ClickHouseConfig.Enabled,
			Logger:           opts.Logger,
			ClickHouse:       opts.ClickHouse,
		},
		RuntimeSite:       opts.RuntimeSite,
		ImageCaptchaStore: opts.ImageCaptchaStore,
		Authenticator:     user_service.NewAuthenticator(opts.DB, opts.Logger, opts.JWT, redisOptions),
	}
}

func runtimeFromContext(c *gin.Context) Runtime {
	if c == nil {
		return Runtime{}
	}

	if value, ok := c.Get("_middleware_runtime"); ok {
		if runtime, ok := value.(Runtime); ok {
			return runtime
		}
	}
	return Runtime{}
}
