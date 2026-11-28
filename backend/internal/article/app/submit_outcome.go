package app

import (
	"myblogx/internal/article/domain"
)

// 提交落点决策（票 302 新规则）。
//
// 旧规则是"站点免审核关闭 → 全部进人工队列"，新规则改成：
//
//   - 站点「文章免审核」开启：全部直接真发布（visible），同步风险扫描也不拦，
//     异步合规判定仍作事后兜底。这是运营的紧急放行手段。
//   - 关闭：提交时同步扫词表 + 评分派生，medium 及以上直接 REJECTED（不给改）；
//     none / low 则以 Published + pending_review 落库 —— 作者立刻拿到"已发布"
//     回执，但对读者暂不可见，等异步模型判定通过才转 visible。
//
// 因此**同步路径不再产生人工待审任务**：人工队列只留异步模型判存疑、
// 判定失败与自动拒的复盘任务。Create 与 Update 共用这一个函数，避免
// "新建"和"编辑重投"两套规则漂移。

// submitOutcome 是一次提交动作的最终落点。
type submitOutcome struct {
	Status     domain.Status
	Visibility domain.Visibility
	// Rejected 为 true 表示同步风险命中，落 REJECTED 且不给改。
	Rejected bool
}

// decideSubmit 把「免审核 / 同步风险 / 质量闸门 / 两个机器判据开关」合成一个落点。
//
// 入参都是事务外已经算好的结论；本函数是纯函数，不读配置、不碰库。
//
// 优先级固定：风险命中 > 质量不达标 > 没有机器判据 ⇒ 直接可见 > 待模型判定。
// 风险命中优先于闸门，因为前者落 REJECTED 不给改，后者落草稿可改可重投。
//
// "没有机器判据"是 2026-11-28 新增的一条分支，它与「文章免审核」**共用同一行代码**：
//
//	!scoreEnabled || !aiComplianceEnabled  ⇒  落 Published + visible
//
// 免审核（skipExamining）之所以也走这里，是因为它表达的是同一件事 ——
// "这次提交没有任何机器判据要等"。两件事合成一条分支而不是各写一条，是因为
// 两条并行的"无门槛放行"路径迟早会漂（一条加了关键词处置检查、另一条忘了），
// 而它们本来就该完全一致。
//
// 注意**关键词处置不在这条链里**：命中即拒 / 退回草稿在过滤那一步就已经短路
// （见 create.go / update.go 的 hitDisposition），根本走不到这里。
// 「文章免审核」也不绕过它 —— 那是作者侧最不能接受的绕过。
func decideSubmit(
	skipExamining, scoreEnabled, aiComplianceEnabled, gateRejected, riskRejected bool,
) submitOutcome {
	if riskRejected {
		return submitOutcome{
			Status:     domain.StatusRejected,
			Visibility: domain.VisibilityVisible,
			Rejected:   true,
		}
	}
	if gateRejected {
		// 质量不达标：即使免审核开启也不放行，文章落草稿，作者改完可立刻重投。
		return submitOutcome{Status: domain.StatusDraft, Visibility: domain.VisibilityVisible}
	}
	// 没有任何机器判据可等（免审核，或评分与 AI 合规两个开关都关 / 关掉其一）：
	// 直接真发布，读者立刻可见。
	if skipExamining || !scoreEnabled || !aiComplianceEnabled {
		return submitOutcome{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}
	}
	// 干净文章：立刻"已发布"，但先对读者隐藏，等模型判定。
	return submitOutcome{
		Status:     domain.StatusPublished,
		Visibility: domain.VisibilityPendingReview,
	}
}
