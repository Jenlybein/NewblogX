package app

import (
	"context"

	aipkg "myblogx/internal/ai"
)

// releaseQuotaLease 归还一次动作级预扣（ADR-0028）。
//
// 只吞掉错误：调用点此刻已经有一条更重要的错误要返回（模型失败 / 数据校验失败），
// 用一个 Redis 报错盖掉它只会让人排查跑偏。归还本身在适配器里是幂等的。
func releaseQuotaLease(ctx context.Context, lease aipkg.QuotaLease) {
	if lease == nil {
		return
	}
	_ = lease.Release(ctx)
}
