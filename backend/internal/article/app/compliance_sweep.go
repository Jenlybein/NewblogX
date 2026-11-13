package app

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// EscalateForReview 把一篇「审核中但没有任何待处理任务」的文章转回人工。
//
// 它只服务于额度延迟重审的兜底（ADR-0028）：正常路径下这类文章次日会被自动
// 重审，不会走到这里；只有延迟事件丢失（库恢复、误删、清洗）时才靠每日扫描
// 把它们捞回来，否则作者看到「审核中」、管理员队列里却永远没有这一行。
//
// 幂等：文章已经不在审核中就什么都不做；已经存在待处理任务也不重复建。
func (s *Service) EscalateForReview(ctx context.Context, articleID domain.ArticleID, source article_review_enum.Source) error {
	if articleID.IsZero() {
		return nil
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		article, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			if errors.Is(err, domain.ErrArticleNotFound) {
				// 文章已被删除：没有可处理的文章，也算"目的已达成"。
				return nil
			}
			return err
		}
		if article.Status() != domain.StatusExamining {
			// 已经被人放行 / 驳回 / 改回草稿：不再插手。
			return nil
		}
		tasks, err := r.Reviews().ListPendingByArticle(ctx, articleID)
		if err != nil {
			return err
		}
		if len(tasks) > 0 {
			// 已经有人在处理，不重复建任务。
			return nil
		}
		return createReviewTask(ctx, s, r, article, review.Source(source), s.clock.Now())
	})
}
