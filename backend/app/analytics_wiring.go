package app

import (
	"context"
	"time"

	"myblogx/internal/analytics/adapter/clickhouse"
	analyticshttp "myblogx/internal/analytics/adapter/http"
	analyticsmysql "myblogx/internal/analytics/adapter/mysql"
	"myblogx/internal/analytics/domain"
	"myblogx/internal/platform/cdc/deadletter"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_site"
	"myblogx/router"

	"gorm.io/gorm"
)

// analyticsLogOptions 组装日志查询所需配置。
//
// 抽成函数是因为 user 模块的"最后登录"也要用同一份配置。
func analyticsLogOptions(infra *Infra) clickhouse.Options {
	return clickhouse.Options{
		LogConfig:         infra.Config.Log,
		SystemConfig:      infra.Config.System,
		ClickHouseEnabled: infra.Config.ClickHouse.Enabled,
		Logger:            infra.Logger,
		ClickHouse:        infra.ClickHouse,
	}
}

// WireAnalytics 装配统计与日志查询模块，并注入路由处理器。
func WireAnalytics(infra *Infra) {
	if infra == nil {
		return
	}
	redisDeps := redis_service.NewDeps(infra.Redis, infra.Logger)
	h := analyticshttp.NewHandler(
		analyticsLogOptions(infra),
		analyticsmysql.NewStats(infra.DB),
		&analyticsFlow{deps: redisDeps},
		&analyticsDeadLetters{db: infra.DB},
	)
	router.SetAnalyticsHandler(h)
}

// analyticsFlow 把 Redis 的访问量计数适配成统计端口。
type analyticsFlow struct {
	deps redis_service.Deps
}

var _ domain.FlowCounter = (*analyticsFlow)(nil)

// Current 返回今天截至目前的总访问量。
func (f *analyticsFlow) Current() int {
	return redis_site.GetFlow(f.deps)
}

// Recent 返回最近 days 天的访问量。
func (f *analyticsFlow) Recent(days int) []domain.FlowPoint {
	list := redis_site.GetRecentFlow(f.deps, days)
	out := make([]domain.FlowPoint, 0, len(list))
	for _, item := range list {
		out = append(out, domain.FlowPoint{Date: item.Date, Count: item.Count})
	}
	return out
}

// analyticsDeadLetters 把平台死信查询适配成统计端口。
type analyticsDeadLetters struct {
	db *gorm.DB
}

var _ domain.DeadLetterReader = (*analyticsDeadLetters)(nil)

// Query 实现 domain.DeadLetterReader。
func (d *analyticsDeadLetters) Query(ctx context.Context, q domain.DeadLetterQuery) ([]domain.DeadLetter, error) {
	_ = ctx
	list, err := deadletter.Query(d.db, deadletter.QueryOption{
		Stream:         q.Stream,
		Status:         q.Status,
		CdcJobID:       q.CdcJobID,
		CdcJobIDPrefix: q.CdcJobIDPrefix,
		CdcJobIDs:      q.CdcJobIDs,
		Limit:          q.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.DeadLetter, 0, len(list))
	for _, item := range list {
		out = append(out, domain.DeadLetter{
			ID:          uint64(item.ID),
			Stream:      item.Stream,
			CdcJobID:    item.CdcJobID,
			SourceTable: item.SourceTable,
			Action:      item.Action,
			TargetKey:   item.TargetKey,
			RetryCount:  item.RetryCount,
			Status:      item.Status,
			ErrorCode:   item.ErrorCode,
			ErrorMsg:    item.ErrorMsg,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return out, nil
}

// 保证 time 包被使用（更新时间格式化在适配器内完成）。
var _ = time.Time{}
