package middleware

import (
	"database/sql"

	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"

	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RuntimeOptions 是"中间件运行时"需要的全部依赖。
//
// 这里刻意只保留 NewRuntime 真正读取的字段：旧版 Options 有 21 个字段，
// 但实际只用到 10 个，多出来的（ES、七牛、上传、邮件、Tx、Version…）
// 都是"顺手塞进来的"，正是 R2 要消灭的东西。
//
// 命名不用 Options：它表达的是"一个组件的构造参数"，
// 而不是"随便抓一把依赖往下传"。
type RuntimeOptions struct {
	System            conf.System
	JWT               conf.Jwt
	Log               conf.Logrus
	ClickHouseConfig  conf.ClickHouse
	Logger            *logrus.Logger
	DB                *gorm.DB
	Redis             *redis.Client
	ClickHouse        *sql.DB
	RuntimeSite       *sitecfg.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
}
