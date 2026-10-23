package log_api

import (
	"database/sql"
	"myblogx/conf"

	"github.com/sirupsen/logrus"
)

type Deps struct {
	Log              conf.Logrus
	System           conf.System
	ClickHouseConfig conf.ClickHouse
	Logger           *logrus.Logger
	ClickHouse       *sql.DB
}

type LogApi struct {
	App Deps
}

func New(deps Deps) LogApi {
	return LogApi{App: deps}
}
