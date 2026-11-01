// 数据库初始化

package core

import (
	"time"

	"myblogx/conf"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

// InitDB 初始化数据库连接
// InitDB 初始化数据库连接
//
// 不再接收 redisClient：它原先只用于给查询缓存插件当后端，插件已关闭
// （理由见文件末尾的注释）。Redis 客户端仍由 InitRedis 单独创建并交给
// 需要它的组件，不该为了一个已废弃的插件穿过本函数。
func InitDB(dbCfg []conf.DB, gormConf conf.GormConf, logConfig conf.Logrus, loggerInstance *logrus.Logger) *gorm.DB {
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

	// 这里**不再**装载 go-gorm/caches 查询缓存插件。
	//
	// 关掉它的三个理由（#132 实测，非推测）：
	//
	//  1. 它的缓存标识只由「SQL + 参数」组成，不含目标结构体类型。两个 SQL
	//     相同、形状不同的查询会共用同一个键，窄行查询存进去的 JSON 被整行
	//     查询命中时，缺失字段**静默归零**（不报错）—— 线上已因此签发过
	//     token_version=0 的访问令牌。
	//
	//  2. 它在 Create/Update/Delete 回调里无条件 Invalidate，而实现是
	//     `SCAN gorm-caches::*` 全表扫描 + DEL：**任何一次写都清空全部缓存**。
	//     本站每次文章浏览都要写浏览计数，写极频繁，缓存刚存进去就被清空，
	//     命中率趋近于 0。
	//
	//  3. 于是它只留下成本：每次写一次 O(N) 的 Redis SCAN，换来几乎为零的
	//     命中率，外加上面那类静默数据损坏。
	//
	// 需要缓存的读接口应显式走应用层缓存（明确键、明确失效），而不是让 ORM
	// 层无差别缓存全部查询。
	//
	// 若将来要重新启用：缓存键必须带上目标结构体形状
	// （platform/mysql.QueryCacheKey），否则缺陷会原样回来。
	// testkit/mysqlcontainer.WithQueryCache 保留了带护栏的装法，可用来验证。

	return db
}
