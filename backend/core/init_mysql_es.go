package core

import (
	"myblogx/conf"
	"myblogx/service/log_service"
	"myblogx/service/river_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type MySQLESDeps struct {
	RiverConfig       conf.River
	LogConfig         conf.Logrus
	System            conf.System
	ClickHouseEnabled bool
	Logger            *logrus.Logger
	DB                *gorm.DB
	ESClient          *elasticsearch.Client
}

func InitMySQLES(deps MySQLESDeps) {
	if !deps.RiverConfig.Enabled {
		deps.Logger.Infof("配置中未启用 MySQL 同步任务")
		return
	}

	r, err := river_service.NewRiver(deps.RiverConfig, deps.Logger, deps.DB, deps.ESClient)
	if err != nil {
		deps.Logger.Fatal(err)
	}
	r.SetLogDeps(log_service.NewDeps(deps.LogConfig, deps.System, deps.ClickHouseEnabled, deps.Logger, nil))

	go r.Run()
}
