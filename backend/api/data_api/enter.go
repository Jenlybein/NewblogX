package data_api

import (
	"database/sql"
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	System           conf.System
	Log              conf.Logrus
	ClickHouseConfig conf.ClickHouse
	Logger           *logrus.Logger
	DB               *gorm.DB
	Redis            *redis.Client
	ClickHouse       *sql.DB
}

type DataApi struct {
	App Deps
}

func New(deps Deps) DataApi {
	return DataApi{App: deps}
}
