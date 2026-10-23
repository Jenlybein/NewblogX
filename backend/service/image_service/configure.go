package image_service

import (
	"myblogx/conf"
	"myblogx/service/redis_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	QiNiu  conf.QiNiu
	Upload conf.Upload
	DB     *gorm.DB
	Redis  *redis.Client
	Logger *logrus.Logger
}

func NewDeps(qiNiu conf.QiNiu, upload conf.Upload, db *gorm.DB, redisClient *redis.Client, logger *logrus.Logger) Deps {
	return Deps{
		QiNiu:  qiNiu,
		Upload: upload,
		DB:     db,
		Redis:  redisClient,
		Logger: logger,
	}
}

func (d Deps) RedisDeps() redis_service.Deps {
	return redis_service.Deps{
		Client: d.Redis,
		Logger: d.Logger,
	}
}
