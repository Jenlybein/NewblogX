package apideps

import (
	"database/sql"

	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Deps 是 API 层可见的显式依赖集合，不暴露运行时容器能力。
type Deps struct {
	Version           string
	ConfigFile        string
	System            conf.System
	JWT               conf.Jwt
	Log               conf.Logrus
	ClickHouseConfig  conf.ClickHouse
	ES                conf.ES
	QQ                conf.QQ
	Email             conf.Email
	QiNiu             conf.QiNiu
	Upload            conf.Upload
	Logger            *logrus.Logger
	DB                *gorm.DB
	Redis             *redis.Client
	ClickHouse        *sql.DB
	ESClient          *elasticsearch.Client
	RuntimeSite       *site_service.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
}
