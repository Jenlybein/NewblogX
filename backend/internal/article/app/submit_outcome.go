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

// decideSubmit 把「免审核开关 / 质量闸门 / 同步风险扫描」合成一个落点。
//
// 入参都是事务外已经算好的结论；本函数是纯函数，不读配置、不碰库。
// 顺序固定：风险命中 > 质量不达标 > 正常放行。风险命中优先于闸门，
// 因为前者落 REJECTED 不给改，后者落草稿可改可重投。
func decideSubmit(skipExamining bool, gateRejected bool, riskRejected bool) submitOutcome {
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
	if skipExamining {
		// 免审核：直接真发布，不等模型。
		return submitOutcome{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}
	}
	// 干净文章：立刻"已发布"，但先对读者隐藏，等模型判定。
	return submitOutcome{
		Status:     domain.StatusPublished,
		Visibility: domain.VisibilityPendingReview,
	}
}
