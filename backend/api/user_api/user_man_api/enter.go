package user_man_api

import (
	"database/sql"
	"myblogx/conf"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB               *gorm.DB
	Log              conf.Logrus
	System           conf.System
	ClickHouseConfig conf.ClickHouse
	Logger           *logrus.Logger
	ClickHouse       *sql.DB
}

type UserManApi struct {
	App Deps
}

func New(deps Deps) UserManApi {
	return UserManApi{App: deps}
}
