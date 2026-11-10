package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// PublishForCompliance 是模型判 clean 后的成功出口（票 302 新规则）。
//
// 文章提交时同步风险扫描没命中 medium 及以上，已以 Published + pending_review
// 落库；作者立刻拿到"已发布"回执，但匿名访客还读不到。模型判干净才在这里
// 真正放开可见。
//
// 幂等：BackToReview / AutoRejectForCompliance 可能已经把文章改走，
// 此时 domain.PublishForCompliance 是空操作，重复投递不会产生额外写入。
func (s *Service) PublishForCompliance(ctx context.Context, articleID domain.ArticleID) error {
	if articleID.IsZero() {
		return domain.ErrArticleNotFound
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if err := a.PublishForCompliance(s.clock.Now()); err != nil {
			return err
		}
		return r.Articles().Update(ctx, a)
	})
}

// DeferForCompliance 把"已发布待可见"的文章改回审核中，但**不建人工任务**。
//
// 它只用于一种结局：作者当日的 AI 合规判定额度已用完，这次判定根本没跑。
// 与 BackToReviewForCompliance 的差别只有一处——不创建待处理任务：
//
//   - 这不是"需要人来判"的情况（明天额度恢复后自动重审）；
//   - 建一条人工任务会让管理员的待处理队列每天多出一批"等一天就会自动过"的行。
//
// 代价是这篇文章在管理员队列里不可见，所以必须有兜底：
// 每日扫描「审核中 + 无待处理任务 + 超过 24 小时」的文章转人工（见 article 的
// EscalateForReview），否则投递丢失就会永久悬挂（ADR-0038）。
//
// 幂等：文章已不在 pending_review 时 domain.BackToReview 返回 ErrInvalidTransition，
// 这里按"目的已达成"吞掉，不把中继推进死信。
func (s *Service) DeferForCompliance(ctx context.Context, articleID domain.ArticleID) error {
	if articleID.IsZero() {
		return domain.ErrArticleNotFound
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if err := a.BackToReview(s.clock.Now()); err != nil {
			if err == domain.ErrInvalidTransition {
				return nil
			}
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		now := s.clock.Now()
		// 撤销原有的待处理任务：额度不足时不该有人工任务挂着这条文章。
		return cancelPendingReviewTasks(ctx, r, articleID, "合规判定额度不足，等待次日自动重审", now)
	})
}

// BackToReviewForCompliance 把"已发布待可见"的文章改回审核中，交人工处置。
//
// 三个入口共用：模型判 suspect、模型判 violation 但风险不足自动拒门槛、
// 以及模型判定重试一次后仍失败。绝不让文章永远停在
// "作者以为发了、读者却看不到"的隐形拒稿状态。
//
// 它在一个事务里做三件事（与 AutoRejectForCompliance 对称）：
//  1. 状态改回 EXAMINING、可见性清回 visible；
//  2. 撤销原有的待处理任务，避免同一篇文章留下两条；
//  3. 新建一条待处理任务，source 点名这次为什么交人。
//
// 幂等：若文章已经不在 pending_review（例如已被管理员处理），
// domain.BackToReview 返回 ErrInvalidTransition，这里按"目的已达成"吞掉，
// 不把中继推进死信。
func (s *Service) BackToReviewForCompliance(ctx context.Context, articleID domain.ArticleID, source article_review_enum.Source) error {
	if articleID.IsZero() {
		return domain.ErrArticleNotFound
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if err := a.BackToReview(s.clock.Now()); err != nil {
			if err == domain.ErrInvalidTransition {
				return nil
			}
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		now := s.clock.Now()
		if err := cancelPendingReviewTasks(ctx, r, articleID, "模型判定未放行，转人工", now); err != nil {
			return err
		}
		return createReviewTask(ctx, s, r, a, review.Source(source), now)
	})
}
