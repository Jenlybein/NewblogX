package ai_api

import (
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB          *gorm.DB
	Logger      *logrus.Logger
	Redis       *redis.Client
	ESClient    *elasticsearch.Client
	ES          conf.ES
	RuntimeSite *site_service.RuntimeConfigService
}

type AIApi struct {
	App Deps
}

func New(deps Deps) AIApi {
	return AIApi{App: deps}
}
