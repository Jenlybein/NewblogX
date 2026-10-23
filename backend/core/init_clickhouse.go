package core

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"myblogx/conf"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

const (
	clickhouseDefaultDatabase = "blogx_logs"
)

func InitClickHouse(cfg *conf.ClickHouse) *sql.DB {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	if len(cfg.Addresses) == 0 {
		panic("clickhouse.addresses 不能为空")
	}

	dialTimeout := time.Duration(cfg.DialTimeout) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	dbName := cfg.Database
	if dbName == "" {
		dbName = clickhouseDefaultDatabase
	}

	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr: cfg.Addresses,
		Auth: clickhouse.Auth{
			Database: dbName,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: dialTimeout,
		Settings: clickhouse.Settings{
			"allow_experimental_object_type": 1,
		},
	})

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle < 0 {
		maxIdle = 0
	}
	conn.SetMaxOpenConns(maxOpen)
	conn.SetMaxIdleConns(maxIdle)
	conn.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		panic(fmt.Errorf("clickhouse 连接失败，请先执行 SQL migration 初始化数据库和表结构: %w", err))
	}

	return conn
}
