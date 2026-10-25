package log_service

import (
	"database/sql"

	"myblogx/conf"

	"github.com/sirupsen/logrus"
)

type Options struct {
	LogConfig        conf.Logrus
	SystemConfig     conf.System
	ClickHouseEnable bool
	Logger           *logrus.Logger
	ClickHouse       *sql.DB
}

func NewOptions(logConfig conf.Logrus, systemConfig conf.System, clickHouseEnable bool, logger *logrus.Logger, clickHouse *sql.DB) Options {
	return Options{
		LogConfig:        logConfig,
		SystemConfig:     systemConfig,
		ClickHouseEnable: clickHouseEnable,
		Logger:           logger,
		ClickHouse:       clickHouse,
	}
}
