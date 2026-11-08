package app

import (
	"context"

	"myblogx/internal/article/domain"
)

// 投稿配额（票 304 / ADR-0031）：普通用户每自然日可提交的次数上限。
//
// 计量单位是**提交动作**，而不是"发布成功的文章"：
//   - 新建并提交审核/发布：1 次；
//   - 更新时显式提交审核：1 次；
//   - 编辑已发布/审核中的文章触发的自动重新送审：同样 1 次 —— 它也会真实产生
//     一条审核任务（以及闸门落地后的一次评分调用），不把它算进去等于留了一条
//     "反复保存即可无限次重审"的绕过路径；
//   - 退回草稿、存草稿、"已发布 + 跳过审核"的幂等提交：0 次（没有产生提交动作）。
//
// 被打回也计（提交已经真实发生）；因系统不可用而失败的提交不计（预占后释放）。
// 管理员豁免是固定规则（spec 决策 19），不随站点配置开关。

// reserveSubmitQuota 为一次提交动作预占额度。
//
// 返回 (nil, nil) 表示本次不占额度：管理员豁免，或装配层没有注入配额端口
// （只有测试装配会这样，见 New 的说明）。
func (s *Service) reserveSubmitQuota(ctx context.Context, actor domain.Viewer) (SubmitQuotaReservation, error) {
	if s.quota == nil || actor.IsAdmin() {
		return nil, nil
	}
	return s.quota.Reserve(ctx, actor.UserID)
}

// releaseSubmitQuota 归还一次预占。
//
// nil 预占、归还失败都不改变已经发生的失败结果：作者看到的应当是"这次提交失败"
// 的原始原因（内存故障 / 数据库故障），把归还的 Redis 报错顶上去只会让人无从下手。
// 归还本身在适配器里是幂等的，所以调用点可以放心同时用显式归还与 defer 兜底。
func (s *Service) releaseSubmitQuota(ctx context.Context, res SubmitQuotaReservation) {
	if res == nil {
		return
	}
	_ = res.Release(ctx)
}
