package app

import (
	"context"
	"time"

	"myblogx/internal/analytics/adapter/clickhouse"
	analyticshttp "myblogx/internal/analytics/adapter/http"
	analyticsmysql "myblogx/internal/analytics/adapter/mysql"
	"myblogx/internal/analytics/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/cdc/dlq"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/platform/cdc/river"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/log/writer"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_site"

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
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	h := analyticshttp.NewHandler(
		analyticsLogOptions(infra),
		analyticsmysql.NewStats(infra.DB),
		&analyticsFlow{deps: redisOptions},
		&analyticsDeadLetters{db: infra.DB},
		&analyticsReplayer{svc: newDeadLetterReplayer(infra)},
	)
	router.SetAnalyticsHandler(h)
}

// newDeadLetterReplayer 组装"死信重投"的编排服务。
//
// 两个执行器分别负责一类 stream：
//   - es_river：用死信里的 ES bulk body 重投（幂等，按文档 ID）
//   - outbox_relay：把发件箱行退回 pending，交给中继按自己的退避重投
//   - image_ref_river：只支持删除类（payload 里有 owner_id + ref_type），
//     新增/更新类的行数据经 JSON 往返后类型不可靠，执行器会明确拒绝
//
// 回放日志通过 Emit 接到统一的日志管道（ClickHouse），这样管理端的
// 「回放日志」页第一次真正有数据，且同一条死信有完整时间线。
func newDeadLetterReplayer(infra *Infra) *dlq.Service {
	logOptions := writer.NewOptions(
		infra.Config.Log,
		infra.Config.System,
		infra.Config.ClickHouse.Enabled,
		infra.Logger,
		infra.ClickHouse,
	)
	return dlq.New(dlq.Options{
		DB:     infra.DB,
		Logger: infra.Logger,
		Emit: func(input writer.ReplayEventInput) {
			writer.EmitReplayEvent(logOptions, input)
		},
		Replayers: []dlq.Replayer{
			river.NewESReplayer(infra.ESClient),
			outboxrelay.NewReplayer(infra.DB),
			imageref.NewReplayer(infra.DB),
		},
	})
}

// analyticsReplayer 把平台层的重投编排适配成统计模块的端口。
type analyticsReplayer struct {
	svc *dlq.Service
}

var _ domain.DeadLetterReplayer = (*analyticsReplayer)(nil)

// SupportedStreams 实现 domain.DeadLetterReplayer。
func (r *analyticsReplayer) SupportedStreams() []string {
	if r == nil || r.svc == nil {
		return nil
	}
	return r.svc.SupportedStreams()
}

// Replay 实现 domain.DeadLetterReplayer。
func (r *analyticsReplayer) Replay(ctx context.Context, ids []ctype.ID) ([]domain.DeadLetterReplayResult, error) {
	if r == nil || r.svc == nil {
		return nil, nil
	}
	results, err := r.svc.Replay(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.DeadLetterReplayResult, 0, len(results))
	for _, item := range results {
		out = append(out, domain.DeadLetterReplayResult{
			ID:      item.ID,
			Stream:  item.Stream,
			Status:  item.Status,
			Result:  item.Result,
			Message: item.Message,
		})
	}
	return out, nil
}

// analyticsFlow 把 Redis 的访问量计数适配成统计端口。
type analyticsFlow struct {
	deps redis_service.Options
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
		Offset:         q.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.DeadLetter, 0, len(list))
	for _, item := range list {
		out = append(out, domain.DeadLetter{
			ID:          item.ID,
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

// Count 实现 domain.DeadLetterReader。
func (d *analyticsDeadLetters) Count(ctx context.Context, q domain.DeadLetterQuery) (int64, error) {
	_ = ctx
	return deadletter.Count(d.db, deadletter.QueryOption{
		Stream:         q.Stream,
		Status:         q.Status,
		CdcJobID:       q.CdcJobID,
		CdcJobIDPrefix: q.CdcJobIDPrefix,
		CdcJobIDs:      q.CdcJobIDs,
	})
}

// 保证 time 包被使用（更新时间格式化在适配器内完成）。
var _ = time.Time{}
