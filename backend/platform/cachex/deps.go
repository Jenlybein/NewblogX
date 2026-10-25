package cachex

import (
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// Options 是缓存访问所需的连接与日志配置。
// 仅承载技术能力，不包含业务语义；它是构造参数，不是往下一路传的依赖袋。
type Options struct {
	Client *redis.Client
	Logger *logrus.Logger
}

func NewOptions(client *redis.Client, logger *logrus.Logger) Options {
	return Options{
		Client: client,
		Logger: logger,
	}
}
