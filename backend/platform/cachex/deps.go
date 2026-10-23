package cachex

import (
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// Deps 是缓存访问所需的最小依赖集。
// 仅承载技术能力，不包含业务语义。
type Deps struct {
	Client *redis.Client
	Logger *logrus.Logger
}

func NewDeps(client *redis.Client, logger *logrus.Logger) Deps {
	return Deps{
		Client: client,
		Logger: logger,
	}
}
