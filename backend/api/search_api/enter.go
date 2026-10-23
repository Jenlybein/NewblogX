package search_api

import (
	"myblogx/conf"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB       *gorm.DB
	Logger   *logrus.Logger
	JWT      conf.Jwt
	Redis    *redis.Client
	ESClient *elasticsearch.Client
	ES       conf.ES
}

type SearchApi struct {
	App Deps
}

func New(deps Deps) SearchApi {
	return SearchApi{App: deps}
}
