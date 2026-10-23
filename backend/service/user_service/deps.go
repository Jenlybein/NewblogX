package user_service

import (
	"myblogx/conf"
	"myblogx/service/redis_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	JWT    conf.Jwt
	Env    string
	DB     *gorm.DB
	Logger *logrus.Logger
	Redis  redis_service.Deps
}

func NewDeps(jwt conf.Jwt, env string, db *gorm.DB, logger *logrus.Logger, redisDeps redis_service.Deps) Deps {
	return Deps{
		JWT:    jwt,
		Env:    env,
		DB:     db,
		Logger: logger,
		Redis:  redisDeps,
	}
}

func NewDepsWithRedis(jwt conf.Jwt, env string, db *gorm.DB, logger *logrus.Logger, redisClient *redis.Client) Deps {
	return NewDeps(jwt, env, db, logger, redis_service.NewDeps(redisClient, logger))
}
