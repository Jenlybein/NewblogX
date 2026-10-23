package category

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
}

type CategoryApi struct {
	App Deps
}

func New(deps Deps) CategoryApi {
	return CategoryApi{App: deps}
}
