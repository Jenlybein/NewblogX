package middleware

import (
	"database/sql"

	"myblogx/conf"
	"myblogx/internal/platform/mysql"
	"myblogx/service/site_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Deps 是 HTTP 装配层共享的显式依赖集合。
//
// 为什么放在 middleware 而不是 api：
//   - api 目录下的各个 handler 包大量 import middleware（取 claims、绑定参数等），
//     因此 api 依赖 middleware 是既成事实；
//   - middleware 及其服务依赖都不 import api，放在这里是叶子位置，不会成环。
//
// 放在这里之后，router（装配）与 api（构造 handler）可以共用同一份依赖，
// 不必像过去那样在 wiring 里把同样的字段拼两遍（apideps.Deps + api.Deps）。
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

	// Tx 是事务边界。业务层（尤其 M4 起的 internal/article/app）应依赖它，
	// 而不是自己去拿 *gorm.DB 开事务。放在这里是为了让装配层只拼一份依赖。
	Tx mysql.TxManager
}
