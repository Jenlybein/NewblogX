// Package redis 是 article 模块的 Redis 适配器：投稿配额（票 304 / ADR-0031）。
//
// 为什么适配器在模块里而不是合成根里：合成根只该做跨模块接线；本文只是把平台层
// 的计数能力翻成 article 用例层定义的端口，属于本模块自己的适配细节。
// 它也是唯一一处把"额度用完"翻成错误码的地方 —— 平台包不认识业务错误码。
package redis

import (
	"context"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/platform/config"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article_quota"
)

// SubmitQuota 实现 app.SubmitQuotaPort。
//
// 上限**每次提交现读**（limit 是闭包而不是快照）：管理员在后台改完
// 「站点 → 文章 → 每日投稿上限」，下一次请求就按新值判，不重启（票 304 验收标准）。
type SubmitQuota struct {
	deps  redis_service.Options
	now   func() time.Time
	limit func() int
}

var _ app.SubmitQuotaPort = (*SubmitQuota)(nil)

// NewSubmitQuota 构造投稿配额适配器。
//
// limit 为 nil 时回落到生效值模块的默认值（15），只为测试与最小装配兜底；
// 生产装配在 backend/app/article_wiring.go 里传生效值的现读闭包。
func NewSubmitQuota(deps redis_service.Options, limit func() int) *SubmitQuota {
	if limit == nil {
		limit = func() int { return config.DefaultArticleSubmitDailyMax }
	}
	return &SubmitQuota{deps: deps, now: time.Now, limit: limit}
}

// Reserve 预占一次当日投稿额度。
//
// 失败关闭：Redis 异常一律返回错误（不放行提交），且**不**冒充"额度用完"——
// 两者对作者的含义完全不同（平台故障 vs 今天没次数了）。
func (q *SubmitQuota) Reserve(_ context.Context, userID domain.UserID) (app.SubmitQuotaReservation, error) {
	reservation, allowed, err := redis_article_quota.ReserveSubmitQuota(q.deps, userID, q.now(), q.limit())
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrSubmitQuotaExceeded
	}
	return &submitQuotaReservation{inner: reservation, deps: q.deps}, nil
}

// submitQuotaReservation 把平台层的预占适配成 app.SubmitQuotaReservation。
type submitQuotaReservation struct {
	inner *redis_article_quota.Reservation
	deps  redis_service.Options
}

// Release 归还这次预占（幂等：重复归还不会放大当日额度）。
func (r *submitQuotaReservation) Release(_ context.Context) error {
	if r == nil || r.inner == nil {
		return nil
	}
	return r.inner.Release(r.deps)
}
