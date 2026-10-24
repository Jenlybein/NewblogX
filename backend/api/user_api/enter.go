package user_api

import (
	"database/sql"
	"myblogx/api/user_api/log_api"
	"myblogx/conf"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Deps 是旧 user_api 剩余部分（登录日志）的依赖。
//
// profile / user_man 已迁到 internal/user，auth 已迁到 internal/auth；
// 这里只剩登录日志查询，按文档归 S4.9 analytics，迁移后本包整体删除。
type Deps struct {
	DB               *gorm.DB
	JWT              conf.Jwt
	Log              conf.Logrus
	System           conf.System
	ClickHouseConfig conf.ClickHouse
	ClickHouse       *sql.DB
	QQ               conf.QQ
	Email            conf.Email
	Logger           *logrus.Logger
	Redis            *redis.Client
}

type UserApi struct {
	LogApi log_api.LogApi
}

func New(deps Deps) UserApi {
	return UserApi{
		LogApi: log_api.New(log_api.Deps{
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
		}),
	}
}
