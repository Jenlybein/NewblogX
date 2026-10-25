// Package http 是统计与日志查询的 HTTP 适配器。
package http

import (
	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/analytics/domain"
)

// Handler 聚合站点统计与日志查询接口。
type Handler struct {
	logs        clickhouse.Options
	stats       domain.StatsRepository
	flow        domain.FlowCounter
	deadLetters domain.DeadLetterReader
}

// NewHandler 构造处理器。
func NewHandler(
	logs clickhouse.Options,
	stats domain.StatsRepository,
	flow domain.FlowCounter,
	deadLetters domain.DeadLetterReader,
) *Handler {
	if stats == nil {
		panic("analytics/http: 统计仓储未注入")
	}
	return &Handler{logs: logs, stats: stats, flow: flow, deadLetters: deadLetters}
}
