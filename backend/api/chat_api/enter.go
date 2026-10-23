package chat_api

import (
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Logger *logrus.Logger
	JWT    conf.Jwt
	Redis  *redis.Client
}

type ChatApi struct {
	App Deps
}

func New(deps Deps) ChatApi {
	return ChatApi{App: deps}
}
