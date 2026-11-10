// Package redis 是 AI 模块的 Redis 适配器：每日额度（ADR-0038）。
//
// 为什么适配器在模块里而不是合成根里：合成根只该做跨模块接线；本文只是把
// 平台层的计数能力翻成 internal/ai 定义的端口，属于本模块自己的适配细节。
// 它也是唯一一处把"额度用完"翻成业务结论的地方（平台包不认识场景与角色）。
package redis

import (
	"context"
	"time"

	"myblogx/internal/ai"
	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_ai_quota"
)

// DailyQuota 实现 ai.QuotaGate。
//
// 它只做计数，不读配置、不认识管理员：上限由出站口每次从现读配置算出后传入，
// 所以后台改完下一次请求即生效。
type DailyQuota struct {
	deps redis_service.Options
	now  func() time.Time
}

var _ ai.QuotaGate = (*DailyQuota)(nil)

// NewDailyQuota 构造每日额度适配器。
func NewDailyQuota(deps redis_service.Options) *DailyQuota {
	return &DailyQuota{deps: deps, now: time.Now}
}

// Reserve 预占一次当日额度。
//
// 失败关闭：Redis 异常一律返回错误（不冒充"额度用完"——两者对用户含义不同：
// 平台故障 vs 今天没次数了）。
func (q *DailyQuota) Reserve(_ context.Context, scene string, userID ctype.ID, limit int) (ai.QuotaLease, bool, error) {
	reservation, allowed, err := redis_ai_quota.ReserveAIDailyQuota(q.deps, scene, userID, q.now(), limit)
	if err != nil {
		return nil, false, err
	}
	if !allowed || reservation == nil {
		// 额度已用完，或该场景不限（平台层已直接放行且没有写键）。
		return nil, allowed, nil
	}
	return dailyQuotaLease{inner: reservation, deps: q.deps}, true, nil
}

// dailyQuotaLease 把平台层的预占适配成 ai.QuotaLease。
type dailyQuotaLease struct {
	inner *redis_ai_quota.Reservation
	deps  redis_service.Options
}

// Release 归还这次预占（幂等）。
func (l dailyQuotaLease) Release(_ context.Context) error {
	return l.inner.Release(l.deps)
}
