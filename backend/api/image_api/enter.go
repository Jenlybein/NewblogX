package image_api

import (
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Logger *logrus.Logger
	QiNiu  conf.QiNiu
	Upload conf.Upload
	Redis  *redis.Client
}

type ImageApi struct {
	App Deps
}

func New(deps Deps) ImageApi {
	return ImageApi{App: deps}
}
