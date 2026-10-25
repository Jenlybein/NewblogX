package ai_api

import (
	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"

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
	RuntimeSite *sitecfg.RuntimeConfigService
}

type AIApi struct {
	App Deps
}

func New(deps Deps) AIApi {
	return AIApi{App: deps}
}
