// 数据库初始化

package core

import (
	"context"
	"fmt"
	"time"

	"myblogx/conf"

	"github.com/go-gorm/caches/v4"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

// 实现 caches.Cacher 接口
type redisCacher struct {
	rdb *redis.Client
}

// Get 从 Redis 中获取缓存数据
func (c *redisCacher) Get(ctx context.Context, key string, q *caches.Query[any]) (*caches.Query[any], error) {
	res, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := q.Unmarshal([]byte(res)); err != nil {
		return nil, err
	}

	return q, nil
}

// Store 将缓存数据存储到 Redis 中
func (c *redisCacher) Store(ctx context.Context, key string, val *caches.Query[any]) error {
	res, err := val.Marshal()
	if err != nil {
		return err
	}

	c.rdb.Set(ctx, key, res, 300*time.Second) // Set proper cache time
	return nil
}

// Invalidate 从 Redis 中删除所有缓存数据
func (c *redisCacher) Invalidate(ctx context.Context) error {
	var (
		cursor uint64
		keys   []string
	)
	for {
		var (
			k   []string
			err error
		)
		k, cursor, err = c.rdb.Scan(ctx, cursor, fmt.Sprintf("%s*", caches.IdentifierPrefix), 0).Result()
		if err != nil {
			return err
		}
		keys = append(keys, k...)
		if cursor == 0 {
			break
		}
	}

	if len(keys) > 0 {
		if _, err := c.rdb.Del(ctx, keys...).Result(); err != nil {
			return err
		}
	}
	return nil
}

// InitDB 初始化数据库连接
func InitDB(dbCfg []conf.DB, gormConf conf.GormConf, logConfig conf.Logrus, loggerInstance *logrus.Logger, redisClient *redis.Client) *gorm.DB {
	if len(dbCfg) == 0 {
		loggerInstance.Fatalf("数据库配置错误：未配置数据库")
	}

	// 配置日志（Debug 模式）
	gormLogConfig := logger.Config{
		SlowThreshold:             time.Second, // 慢查询阈值（超过 1 秒标红）
		LogLevel:                  logger.Warn, // SQL 日志级别（Debug 核心）
		Colorful:                  logConfig.StdoutFormat == "text",
		IgnoreRecordNotFoundError: true, // 忽略记录不存在错误

	}
	if gormConf.Debug {
		gormLogConfig.LogLevel = logger.Info
	}
	newLogger := logger.New(
		loggerInstance,
		gormLogConfig,
	)

	gormCfg := gorm.Config{
		Logger:                                   newLogger, // 配置日志
		DisableForeignKeyConstraintWhenMigrating: true,      // 禁用外键约束
		TranslateError:                           true,      // 翻译错误
	}

	// 从配置文件中读取数据库配置
	DB := dbCfg[0] // 写库
	dsn := DB.DSN()

	// 连接数据库（使用主库初始化）
	db, err := gorm.Open(mysql.Open(dsn), &gormCfg)
	if err != nil {
		loggerInstance.Fatalf("数据库连接失败: %s", err)
	}

	loggerInstance.Infof("数据库连接成功 %s", DB.SafeDSN())

	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(gormConf.MaxIdleConns)
	sqlDB.SetMaxOpenConns(gormConf.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour * time.Duration(gormConf.ConnMaxLifetime))

	// 读写分离配置
	if len(dbCfg) > 1 {
		var readList []gorm.Dialector
		for _, d := range dbCfg[1:] {
			readList = append(readList, mysql.Open(d.DSN()))
		}
		// 读库不为空，则注册读写分离的配置
		err := db.Use(dbresolver.Register(dbresolver.Config{
			Sources:  []gorm.Dialector{mysql.Open(DB.DSN())}, // 写库（主库）
			Replicas: readList,                               // 读库（从库）
			Policy:   dbresolver.RandomPolicy{},
		}))
		if err != nil {
			loggerInstance.Fatalf("数据库读写分离配置失败: %s", err)
		}
		loggerInstance.Infof("数据库读写分离配置成功 %d 个读库", len(readList))
	}

	// 缓存加速配置
	cachesPlugin := &caches.Caches{Conf: &caches.Config{
		Easer:  true,
		Cacher: &redisCacher{rdb: redisClient},
	}}
	if err := db.Use(cachesPlugin); err != nil {
		loggerInstance.Fatalf("数据库缓存插件配置失败: %s", err)
	}
	loggerInstance.Infof("数据库缓存插件配置成功")

	return db
}
