package writer

import (
	"time"
)

// 本文件只保留「启动时预热日志文件」这一件事。
//
// 原来的 jsonLineSink（同步 json.Marshal + 每写一条就落一次文件）已经删掉：
// 现在所有结构化日志统一走 pipeline.go 的有界异步管道，
// 落文件的实现在 sink_daily.go，落 ClickHouse 的实现在 platform/log/persist。

// EnsureDailyLogFiles 提前创建当天各类日志文件，避免采集器在无文件时持续告警。
// 服务启动时调用；未启用 ClickHouse 时这是"文件已就位"的保证。
func EnsureDailyLogFiles(deps Options) error {
	now := time.Now()
	rootDir := ResolveLogDir(runtimeLogDir(deps))
	appName := ResolveLogApp("", deps.LogConfig.App)

	for _, dirName := range []string{
		LoginEventLogDirName,
		ActionAuditLogDirName,
		CdcEventLogDirName,
		ReplayEventLogDirName,
	} {
		if err := touchDailyFile(rootDir, dirName, appName, now); err != nil {
			return err
		}
	}
	return nil
}

// runtimeLogDir 返回配置中的日志根目录（未配置时由 ResolveLogDir 补默认值）。
func runtimeLogDir(deps Options) string {
	return deps.LogConfig.Dir
}
