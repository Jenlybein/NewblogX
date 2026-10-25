package user_service

import (
	"myblogx/conf"
	"myblogx/internal/platform/redis"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Options struct {
	JWT    conf.Jwt
	Env    string
	DB     *gorm.DB
	Logger *logrus.Logger
	Redis  redis_service.Options
}

func NewOptions(jwt conf.Jwt, env string, db *gorm.DB, logger *logrus.Logger, redisOptions redis_service.Options) Options {
	return Options{
		JWT:    jwt,
		Env:    env,
		DB:     db,
		Logger: logger,
		Redis:  redisOptions,
	}
}

func NewOptionsWithRedis(jwt conf.Jwt, env string, db *gorm.DB, logger *logrus.Logger, redisClient *redis.Client) Options {
	return NewOptions(jwt, env, db, logger, redis_service.NewOptions(redisClient, logger))
}
