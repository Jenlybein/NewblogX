package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/review"
)

// 管理员放行留痕（票「管理员豁免质量闸门」）。
//
// 管理员点"通过"时**豁免质量闸门**：管理员是终审，强制卡他会让紧急放行失效。
// 但豁免不许静默——任务行上要留下"当时多少分、放行线多少"，否则"越级放行"
// 与"正常放行"在库里长得一模一样，复盘与调参都无从下手。
//
// 这条留痕**不影响判定**：分数只是为了写进备注。所以读分数失败（端口未装配、
// 数据库抖动、模型调用失败）一律按"未知分数"降级落库，绝不让管理员放行失败。

// approveNoteFor 拼出这次放行要写进任务行的那句话。
//
// 分数来源与审核队列同一条端口（ModerationScorePort.LatestScores），
// 不另开一条读路径：队列里看到的分数就是放行时记下的分数，两处不会各说各话。
// 放行线来自本次请求的闸门配置快照（QualityGatePolicy.PublishLine），
// 闸门关闭时也照算——它只用于展示"当前的线"，不参与判定。
//
// 幂等由 domain 的状态判断兜住：同一任务重复通过会在 review.Task.Approve
// 处报 ErrTaskHandled，不会写出第二条留痕。
func (s *Service) approveNoteFor(
	ctx context.Context, articleID domain.ArticleID, gate QualityGatePolicy,
) string {
	var score *int
	if s.scores != nil {
		facts, err := s.scores.LatestScores(ctx, []domain.ArticleID{articleID})
		if err == nil {
			if got, ok := facts[articleID]; ok {
				// 有评分记录就是有分数：0 分要原样记成 0 分，只有 nil 才是"取不到"。
				v := got.TotalScore
				score = &v
			}
		}
		// 读失败不回滚、不报错：这条读数是留痕的唯一来源，不是放行的前提。
		// 备注里会写成"未知分数"，所以降级本身也是留痕的一部分（见 FormatApproveNote）。
	}
	return review.FormatApproveNote(score, gate.PublishLine())
}

// reviewTaskScore 把一次批量读出的评分事实收敛成"这条任务的 *int"。
//
// 审核队列要把**当前**总分带给前端（放行确认框显示"当前 X 分（放行线 Y 分）"），
// 所以按文章 ID 取 map 里的 TotalScore。缺记录时保持 nil——
// 它与"模型评了 0 分"不是一回事，前端据此区分"还没有分数"和"分数很差"。
func reviewTaskScore(facts map[domain.ArticleID]moderation.ScoreFacts, articleID domain.ArticleID) *int {
	got, ok := facts[articleID]
	if !ok {
		return nil
	}
	v := got.TotalScore
	return &v
}
