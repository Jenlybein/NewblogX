package dbx

import (
	"myblogx/conf"
	"myblogx/core"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Init 负责初始化数据库连接与读写分离能力。
func Init(dbCfg []conf.DB, gormConf conf.GormConf, logConfig conf.Logrus, logger *logrus.Logger, redisClient *redis.Client) *gorm.DB {
	return core.InitDB(dbCfg, gormConf, logConfig, logger, redisClient)
}
