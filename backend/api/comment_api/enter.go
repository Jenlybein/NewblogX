package comment_api

import (
	"myblogx/service/site_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB          *gorm.DB
	Logger      *logrus.Logger
	Redis       *redis.Client
	RuntimeSite *site_service.RuntimeConfigService
}

type CommentApi struct {
	App Deps
}

func New(deps Deps) CommentApi {
	return CommentApi{App: deps}
}
