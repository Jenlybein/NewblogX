package middleware

import (
	"myblogx/apideps"
	"myblogx/conf"
	"myblogx/service/log_service"
	"myblogx/service/redis_service"
	"myblogx/service/site_service"
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
	Redis             redis_service.Deps
	Log               log_service.Deps
	RuntimeSite       *site_service.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
	Authenticator     *user_service.Authenticator
}

func NewRuntime(deps apideps.Deps) Runtime {
	redisDeps := redis_service.Deps{
		Client: deps.Redis,
		Logger: deps.Logger,
	}
	return Runtime{
		JWT:       deps.JWT,
		LogConfig: deps.Log,
		Logger:    deps.Logger,
		Redis:     redisDeps,
		Log: log_service.Deps{
			LogConfig:        deps.Log,
			SystemConfig:     deps.System,
			ClickHouseEnable: deps.ClickHouseConfig.Enabled,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
		},
		RuntimeSite:       deps.RuntimeSite,
		ImageCaptchaStore: deps.ImageCaptchaStore,
		Authenticator:     user_service.NewAuthenticator(deps.DB, deps.Logger, deps.JWT, redisDeps),
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
