package cachex

import (
	"myblogx/conf"
	"myblogx/core"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// Init 负责初始化 Redis 客户端。
func Init(redisCfg *conf.Redis, logger *logrus.Logger) *redis.Client {
	return core.InitRedis(redisCfg, logger)
}
