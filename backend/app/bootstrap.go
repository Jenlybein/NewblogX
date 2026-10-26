package app

import (
	"context"
	"database/sql"
	"fmt"

	"myblogx/conf"
	"myblogx/core"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/es/articlesync"
	"myblogx/internal/platform/log/writer"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Infra 是启动后可复用的基础设施句柄集合。
type Infra struct {
	Version    string
	ConfigFile string
	Config     *conf.Config

	Logger      *logrus.Logger
	DB          *gorm.DB
	Redis       *redis.Client
	ESClient    *elasticsearch.Client
	ClickHouse  *sql.DB
	RuntimeSite *sitecfg.RuntimeConfigService
}

// InitInfra 初始化不依赖业务表的基础设施。
func InitInfra(cfg *conf.Config, configFile string, version string) (*Infra, error) {
	if cfg == nil {
		return nil, fmt.Errorf("初始化基础设施失败: 配置不能为空")
	}
	normalizeConfig(cfg)

	if err := core.InitSnowflake(cfg); err != nil {
		return nil, fmt.Errorf("初始化基础设施失败: %w", err)
	}

	logger := core.InitLogrus(&cfg.Log, &cfg.System)

	// IP 归属地库。之前这里漏了这一步，而 requestmeta 一直在调 GetAddr，
	// 于是登录日志和用户列表的 addr 恒为「未知地址」。
	// 失败只告警不中断：addr 是展示字段，缺库时 GetAddr 自己会降级。
	if err := core.InitIPDB(); err != nil {
		logger.Warnf("IP 归属地库加载失败，addr 将显示为「未知地址」: %v", err)
	}

	redisClient := core.InitRedis(&cfg.Redis, logger)
	db := core.InitDB(cfg.DB, cfg.GORM, cfg.Log, logger, redisClient)
	clickHouse := core.InitClickHouse(&cfg.ClickHouse)
	esClient := core.EsConnect(&cfg.ES, logger)

	logOptions := writer.Options{
		LogConfig:        cfg.Log,
		SystemConfig:     cfg.System,
		ClickHouseEnable: cfg.ClickHouse.Enabled,
		Logger:           logger,
		ClickHouse:       clickHouse,
	}

	if err := writer.EnsureDailyLogFiles(logOptions); err != nil {
		logger.Errorf("初始化结构化日志文件失败: %v", err)
	}

	// 安装结构化日志管道：五类日志各一条有界队列。
	// 启用 ClickHouse 时直接写表、失败落本地 WAL；未启用时写每日文件
	// （与改造前的落点完全一致，外部采集器无需调整）。
	if err := writer.InitStructuredLog(writer.PipelineConfig{
		Options:    logOptions,
		WALDir:     cfg.Log.WALDir,
		ClickHouse: cfg.ClickHouse.Enabled,
	}); err != nil {
		return nil, fmt.Errorf("初始化结构化日志管道失败: %w", err)
	}

	// 把索引名注入投影写入侧，让"写"和"读"（搜索/AI 检索/CLI 建索引）用同一个名字。
	//
	// 这一步不能省：写入曾经硬编码 "article_index"，两边默认值恰好相同才没出事；
	// 一旦 `es.index` 配成别的值，就会"建在新索引、搜在新索引、增量写进旧索引"，
	// 而且不报错。只在这里注入一次，之后全链路只读（见 articlesync.SetIndexName 的说明）。
	articlesync.SetIndexName(cfg.ES.Index)

	return &Infra{
		Version:    version,
		ConfigFile: configFile,
		Config:     cfg,
		Logger:     logger,
		DB:         db,
		Redis:      redisClient,
		ESClient:   esClient,
		ClickHouse: clickHouse,
	}, nil
}

// InitRuntimeServices 初始化依赖业务表的运行时服务。
func InitRuntimeServices(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	runtimeSite, err := core.InitRuntimeSite(infra.Config, infra.Logger, infra.DB, infra.ConfigFile)
	if err != nil {
		return fmt.Errorf("运行时站点配置初始化失败: %w", err)
	}
	infra.RuntimeSite = runtimeSite
	return nil
}

// RegisterInfraCleanup 把基础设施句柄注册为停机钩子。
//
// 注册顺序很重要：Lifecycle 按「后注册先执行」的顺序运行钩子，
// 因此先在这里注册 DB / Redis / ClickHouse，HTTP 服务随后在 WireHTTP 中注册；
// 停机时 HTTP 会先 Shutdown（等在途请求结束），再关闭这些连接池。
//
// 日志管道最后注册，所以最先执行：必须把队列刷完、断开出口之后，
// 才能关掉 ClickHouse 连接，否则最后一批日志会写进已关闭的连接池。
func RegisterInfraCleanup(lc *Lifecycle, infra *Infra) {
	if lc == nil || infra == nil {
		return
	}

	lc.OnStop("mysql", func(context.Context) error {
		if infra.DB == nil {
			return nil
		}
		sqlDB, err := infra.DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	})

	lc.OnStop("redis", func(context.Context) error {
		if infra.Redis == nil {
			return nil
		}
		return infra.Redis.Close()
	})

	lc.OnStop("clickhouse", func(context.Context) error {
		if infra.ClickHouse == nil {
			return nil
		}
		return infra.ClickHouse.Close()
	})

	// 日志管道最后注册 → 停机时最先执行：先把队列里剩下的日志刷完。
	lc.OnStop("structured-log", func(ctx context.Context) error {
		return writer.CloseStructuredLog(ctx)
	})
}

func validateInfra(infra *Infra) error {
	if infra == nil {
		return fmt.Errorf("infra 不能为空")
	}
	if infra.Config == nil {
		return fmt.Errorf("infra.Config 不能为空")
	}
	if infra.Logger == nil {
		return fmt.Errorf("infra.Logger 不能为空")
	}
	return nil
}
