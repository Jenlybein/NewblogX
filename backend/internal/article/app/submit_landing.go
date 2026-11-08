package app

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// 提交落点的共用副作用（票 302）。
//
// Create 与 Update 都要做两件完全相同的事：给"同步风险直接拒"留一条复盘任务，
// 以及按最终状态选一个领域事件。以前各写一份，改一处忘了另一处就会出现
// "新建拒了有复盘任务、编辑重投拒了没有"这类不对称。

// saveRiskRejectionTask 给一次同步风险直接拒留一条待处理复盘任务。
//
// 误判要看得见，但作者侧只拿到固定文案、不给改。干净文章不在这里建任务：
// 同步路径不产生人工队列，人工任务只由异步模型判定产生。
func saveRiskRejectionTask(ctx context.Context, s *Service, r Repos, a *domain.Article, now time.Time) error {
	return r.Reviews().Save(ctx, review.NewTask(review.NewTaskParams{
		ID:            ctype.NewID(s.ids.NextReviewTaskID()),
		ArticleID:     a.ID(),
		AuthorID:      a.AuthorID(),
		ArticleTitle:  a.Title(),
		PublishStatus: a.Status(),
		Source:        review.Source(article_review_enum.SourceRiskCheck),
		Now:           now,
	}))
}

// submitEventName 按文章的最终状态选领域事件。
//
//  - Published（含 pending_review）→ Published：一次触发评分、合规判定与
//    "审核通过"通知，正好实现"提交后立刻告诉作者已发布"；
//  - Rejected（同步风险命中）→ Rejected：通知链路据此给出固定文案；
//  - Examining → Submitted；Draft → BackToDraft（仅 Update 会走到）；
//  - 其余 → Created（Create 存草稿）/ Updated（Update 的纯内容编辑）。
//
// isCreate 区分"新建"与"更新"：两者的兜底事件不同，其余分支共用。
func submitEventName(status domain.Status, riskRejected, explicitDraft, isCreate bool) string {
	switch {
	case explicitDraft:
		return EventArticleBackToDraft
	case riskRejected:
		return EventArticleRejected
	case status == domain.StatusPublished:
		return EventArticlePublished
	case status == domain.StatusExamining:
		return EventArticleSubmitted
	case isCreate:
		return EventArticleCreated
	default:
		return EventArticleUpdated
	}
}
