package core

import (
	"context"
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

func InitRedis(redisCfg *conf.Redis, logger *logrus.Logger) *redis.Client {
	redisDB := redis.NewClient(&redis.Options{
		Addr:     redisCfg.GetAddr(),
		Username: redisCfg.Username,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})

	_, err := redisDB.Ping(context.Background()).Result()
	if err != nil {
		if logger != nil {
			logger.Fatalf("Redis 连接失败: %v", err)
		}
		panic(err)
	}

	if logger != nil {
		logger.Infof("Redis 连接成功: %s", redisDB.Options().Addr)
	}

	return redisDB
}
