package auth_api

import (
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB          *gorm.DB
	Email       conf.Email
	JWT         conf.Jwt
	Logger      *logrus.Logger
	QQ          conf.QQ
	Redis       *redis.Client
	RuntimeSite *site_service.RuntimeConfigService
	System      conf.System
}

type AuthApi struct {
	App Deps
}

func New(deps Deps) AuthApi {
	return AuthApi{App: deps}
}
