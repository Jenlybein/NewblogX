package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
)

// 提交时的同步风险扫描（票 302 新规则 / ADR-0032 决策 6）。
//
// 与"审核列表标红"用同一份 moderation.Evaluate：只由**评分派生层**（低分 /
// 维度塌陷 / 篇幅异常）驱动。之所以复用，是为了让"提交那一刻判出来的等级"和
// "管理员在列表里看到的等级"永远是同一个口径 —— 两份规则迟早会漂移，漂移的
// 后果是"提交时说干净、队列里却标红"。
//
// 命中词不再参与这里：它们在落库前已被全站内容过滤替换成 `*`，不会让投稿被拒
// （ADR-0032 决策 1）。这里读到的正文已经是替换后的文本。
//
// 时机很关键：这一步在**写事务之前**跑，因为它决定文章落 REJECTED 还是
// Published + pending_review，必须在落库前就有结论。评分派生层读的是已有
// 评分记录（ModerationScorePort），新建文章通常还没有评分记录，此时不产生
// 任何信号、不报错（与票 311 验收标准 5 一致）。
//
// 阈值只认 medium 及以上（含 high）：low 与 none 放行进"已发布待可见"。
// 这是用户明确选定的口径 —— 低风险只提示，不拦人。评分派生层里只有"低分"
// 是 medium，维度塌陷与篇幅异常是 low。

// riskVerdict 是同步扫描的内部结论。
type riskVerdict struct {
	// Rejected 为 true 表示命中 medium 及以上，本次提交直接落 REJECTED。
	Rejected bool
}

// scanSubmitRisk 在提交事务之外同步扫描一次风险。
//
// articleID 非零表示这是一篇已落库的文章（更新/重投），可以读已有评分记录；
// 为零表示新建文章，这次还没有评分记录，不会产生任何信号。
//
// 端口为 nil 时按"没有评分数据"处理，与审核列表的退化行为一致：
// 不能因为没接模型就让整站投稿失败。扫描本身不调用 AI，只做一次评分记录读取，
// 因此不违反"评分调用在事务之外"这条纪律。
func (s *Service) scanSubmitRisk(ctx context.Context, articleID domain.ArticleID) (riskVerdict, error) {
	var facts *moderation.ScoreFacts
	if !articleID.IsZero() && s.scores != nil {
		got, err := s.scores.LatestScores(ctx, []domain.ArticleID{articleID})
		if err != nil {
			// 读评分失败不等于"干净"：拿不到数据就当作拿不到，不能放行。
			// 失败关闭，由调用方拒绝提交并归还配额。
			return riskVerdict{}, apperr.Wrap(apperr.CodeArticleScoreUnavailable, err)
		}
		if score, ok := got[articleID]; ok {
			facts = &score
		}
	}

	result := moderation.Evaluate(facts)
	return riskVerdict{
		Rejected: result.Level.Rank() >= moderation.LevelMedium.Rank(),
	}, nil
}

// newRiskRejection 把同步扫描的结论包成带错误码的错误。
//
// 作者侧只拿到一条固定文案（apperr 的 code 映射），维度、等级都
// 不进响应 —— 这与"作者永远看不到模型判定细节"是同一条边界（ADR-0024 决策 7）。
func newRiskRejection() error {
	return apperr.New(apperr.CodeArticleRiskRejected)
}
