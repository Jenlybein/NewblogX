package core

import (
	"myblogx/conf"
	"myblogx/internal/platform/cdc/river"
	"myblogx/internal/platform/safego"
	"myblogx/service/log_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type MySQLESOptions struct {
	RiverConfig       conf.River
	LogConfig         conf.Logrus
	System            conf.System
	ClickHouseEnabled bool
	Logger            *logrus.Logger
	DB                *gorm.DB
	ESClient          *elasticsearch.Client
}

func InitMySQLES(deps MySQLESOptions) {
	if !deps.RiverConfig.Enabled {
		deps.Logger.Infof("配置中未启用 MySQL 同步任务")
		return
	}

	r, err := river.NewRiver(deps.RiverConfig, deps.Logger, deps.DB, deps.ESClient)
	if err != nil {
		deps.Logger.Fatal(err)
	}
	r.SetLogOptions(log_service.NewOptions(deps.LogConfig, deps.System, deps.ClickHouseEnabled, deps.Logger, nil))

	safego.Go(func(recovered any, stack []byte) {
		deps.Logger.Errorf("MySQL→ES 同步 panic: %v\n%s", recovered, stack)
	}, func() {
		_ = r.Run()
	})
}
