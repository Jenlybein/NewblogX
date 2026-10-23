package top

import (
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Logger *logrus.Logger
	Redis  *redis.Client
}

type TopApi struct {
	App Deps
}

func New(deps Deps) TopApi {
	return TopApi{App: deps}
}
