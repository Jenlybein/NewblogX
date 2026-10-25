// Package clickhouse 是日志查询的 ClickHouse 适配层。
//
// 只负责"按条件查日志"，不负责写入——写入在 platform/log 的日志管道里。
package clickhouse

import (
	"database/sql"

	"myblogx/conf"

	"github.com/sirupsen/logrus"
)

// Options 是查询日志所需的连接与配置。
//
// 不叫 Deps：架构守卫禁止业务模块再出现"Deps 打包结构体"，
// 依赖应该按能力拆分；这里三个字段都是同一条链路（ClickHouse 连接 + 配置）的必要信息，
// 拆开只会让每个调用点多传三个参数。
type Options struct {
	LogConfig         conf.Logrus
	SystemConfig      conf.System
	ClickHouseEnabled bool
	Logger            *logrus.Logger
	ClickHouse        *sql.DB
}
