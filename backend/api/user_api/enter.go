package user_api

import (
	"database/sql"
	"myblogx/api/user_api/auth_api"
	"myblogx/api/user_api/log_api"
	"myblogx/api/user_api/profile_api"
	"myblogx/api/user_api/user_man_api"
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

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
	RuntimeSite      *site_service.RuntimeConfigService
}

type UserApi struct {
	ProfileApi profile_api.ProfileApi
	AuthApi    auth_api.AuthApi
	LogApi     log_api.LogApi
	UserManApi user_man_api.UserManApi
}

func New(deps Deps) UserApi {
	return UserApi{
		ProfileApi: profile_api.New(profile_api.Deps{
			DB:     deps.DB,
			JWT:    deps.JWT,
			Logger: deps.Logger,
			Redis:  deps.Redis,
			System: deps.System,
		}),
		AuthApi: auth_api.New(auth_api.Deps{
			DB:          deps.DB,
			Email:       deps.Email,
			JWT:         deps.JWT,
			Logger:      deps.Logger,
			QQ:          deps.QQ,
			Redis:       deps.Redis,
			RuntimeSite: deps.RuntimeSite,
			System:      deps.System,
		}),
		LogApi: log_api.New(log_api.Deps{
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
		}),
		UserManApi: user_man_api.New(user_man_api.Deps{
			DB:               deps.DB,
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
		}),
	}
}
