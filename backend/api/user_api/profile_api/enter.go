package profile_api

import (
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	JWT    conf.Jwt
	Logger *logrus.Logger
	Redis  *redis.Client
	System conf.System
}

type ProfileApi struct {
	App Deps
}

func New(deps Deps) ProfileApi {
	return ProfileApi{App: deps}
}
