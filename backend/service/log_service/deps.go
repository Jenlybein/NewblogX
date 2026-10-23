package log_service

import (
	"database/sql"

	"myblogx/conf"

	"github.com/sirupsen/logrus"
)

type Deps struct {
	LogConfig        conf.Logrus
	SystemConfig     conf.System
	ClickHouseEnable bool
	Logger           *logrus.Logger
	ClickHouse       *sql.DB
}

func NewDeps(logConfig conf.Logrus, systemConfig conf.System, clickHouseEnable bool, logger *logrus.Logger, clickHouse *sql.DB) Deps {
	return Deps{
		LogConfig:        logConfig,
		SystemConfig:     systemConfig,
		ClickHouseEnable: clickHouseEnable,
		Logger:           logger,
		ClickHouse:       clickHouse,
	}
}
