package middleware

import (
	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/redis"

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
	Log               writer.Options
	RuntimeSite       *sitecfg.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
	Verifier          TokenVerifier
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
		Log: writer.Options{
			LogConfig:        opts.Log,
			SystemConfig:     opts.System,
			ClickHouseEnable: opts.ClickHouseConfig.Enabled,
			Logger:           opts.Logger,
			ClickHouse:       opts.ClickHouse,
		},
		RuntimeSite:       opts.RuntimeSite,
		ImageCaptchaStore: opts.ImageCaptchaStore,
		Verifier:          opts.Verifier,
	}
}

// logConfig 返回当前生效的日志配置。
//
// 优先读运行时配置：后台改"记录请求访问日志"或堆栈采集策略后应当立刻生效，
// 不该等到下一次重启。RuntimeSite 没装配时（部分单测直接构造 Runtime）
// 退回启动时的快照。
func (h Runtime) logConfig() conf.Logrus {
	if h.RuntimeSite != nil {
		return h.RuntimeSite.GetRuntimeLog()
	}
	return h.LogConfig
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
