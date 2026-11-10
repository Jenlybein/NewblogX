package ai

import (
	"context"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 每日额度（ADR-0038）。
//
// 收费点是**唯一真正发模型请求的那两个函数**（Chat / ChatStream）：业务侧看不到
// 计数，新增调用点自动被覆盖。归属信息来自 ctx 上的 CallMeta（场景 + 用户），
// 由 handler 与用例层各自补齐。
//
// 三条跳过规则，任一成立就不计数：
//   - 管理员（meta.IsAdmin）：固定规则，不随站点开关变化；
//   - 没有用户归属（meta.UserID 为零）：异步补评分这类系统链路；
//   - 已经预扣过（meta.Billing 非 BillingNormal）：一次动作内的后续调用。
//
// 额度用尽不是故障：返回 ai.daily_limit_exceeded，调用方必须把它当成正常业务
// 结论（不是重试、不是死信）。额度服务本身坏了才是故障，此时失败关闭。

// QuotaLease 是一次已占用的每日额度。
type QuotaLease interface {
	// Release 归还本次占用。必须幂等：调用点常写成"显式释放 + defer 兜底"。
	Release(ctx context.Context) error
}

// QuotaGate 是每日额度的计数端口。
//
// 接口在使用方定义、由装配层接到 Redis 适配器上：internal/ai 不 import
// platform/redis，单测里换成一个内存假实现即可。
type QuotaGate interface {
	// Reserve 为 (scene, userID) 预占一次当日额度。
	// 返回 (lease, true, nil) 成功；(nil, false, nil) 额度已用完；
	// (nil, false, err) 额度服务异常（调用方失败关闭）。
	Reserve(ctx context.Context, scene string, userID ctype.ID, limit int) (QuotaLease, bool, error)
}

// quotaGate 是进程级单例，与 callReporter 同类：由装配层安装一次，
// 为 nil 时全部空转（单测与命令行工具不关心计数）。
var quotaGate QuotaGate

// SetQuotaGate 安装额度计数端口。只应该在装配层调一次。
func SetQuotaGate(g QuotaGate) { quotaGate = g }

// SceneDailyLimit 把场景常量映射到配置里的具名字段。
//
// 映射放在这里而不是 conf：conf 是最底层包、import 不了 internal/ai，
// 而场景名只允许在 callmeta.go 声明一次（否则日志维度会悄悄漂移）。
func SceneDailyLimit(limits conf.DailyLimits, scene string) int {
	switch scene {
	case SceneScoring:
		return limits.Scoring
	case SceneMetainfo:
		return limits.Metainfo
	case SceneOverwrite:
		return limits.Overwrite
	case SceneDiagnose:
		return limits.Diagnose
	case SceneSearchRewrite:
		return limits.SearchRewrite
	case SceneSearchAnswer:
		return limits.SearchAnswer
	case SceneModeration:
		return limits.Moderation
	default:
		// 未知场景不猜额度：不打标的调用不计费，与"没标注"的日志口径一致。
		return 0
	}
}

// chargeCall 在出站口为一次模型调用预占额度。
//
// 返回 (nil, nil) 表示本次不需要计数（管理员 / 无归属 / 已预扣 / 该场景不限）。
func chargeCall(ctx context.Context, aiConf conf.AI) (QuotaLease, error) {
	meta := callMetaFrom(ctx)
	if meta.IsAdmin || meta.UserID.IsZero() || meta.Billing != BillingNormal {
		return nil, nil
	}
	return reserveQuota(ctx, aiConf, meta.Scene, meta.UserID, false)
}

// BuyActionQuota 为一次「动作」预占 1 次额度。
//
// 用于一次动作内部会多次调用模型的场景（目前只有评分：长文按段落分块，
// 一次投稿可能顺序调 2..N 次）。入口预扣一次，并把 ctx 标成预扣态，
// 内部分块子调用看到标记就不再重复扣（ADR-0038 决策）。
//
// 返回 (nil, nil) 表示本次不需要计数（管理员 / 无归属 / 该场景不限）。
func BuyActionQuota(ctx context.Context, aiConf conf.AI, scene string, userID ctype.ID, isAdmin bool) (QuotaLease, error) {
	if isAdmin || userID.IsZero() {
		return nil, nil
	}
	return reserveQuota(ctx, aiConf, scene, userID, isAdmin)
}

// reserveQuota 是两条入口共用的预占实现。
func reserveQuota(ctx context.Context, aiConf conf.AI, scene string, userID ctype.ID, isAdmin bool) (QuotaLease, error) {
	if isAdmin || userID.IsZero() || quotaGate == nil {
		return nil, nil
	}
	limit := SceneDailyLimit(aiConf.DailyLimits, scene)
	if limit <= 0 {
		// 0 = 不限：连计数都不写，避免给不限的场景白写一个键。
		return nil, nil
	}
	lease, allowed, err := quotaGate.Reserve(ctx, scene, userID, limit)
	if err != nil {
		// 失败关闭：额度服务不可用时拒绝这次调用，但错误码要与"用完了"分开。
		return nil, apperr.Wrap(apperr.CodeAIQuotaUnavailable, err)
	}
	if !allowed {
		return nil, apperr.New(apperr.CodeAIDailyLimitExceeded)
	}
	return lease, nil
}

// releaseQuota 归还预占；nil 与重复调用都安全。
//
// 失败只忽略：调用点已经有一条更重要的错误要返回（模型失败 / 请求失败），
// 用一个 Redis 报错盖掉它只会让排查跑偏。
func releaseQuota(ctx context.Context, lease QuotaLease) {
	if lease == nil {
		return
	}
	_ = lease.Release(ctx)
}
