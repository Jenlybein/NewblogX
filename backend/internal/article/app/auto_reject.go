package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// AutoRejectNote 是系统自动拒绝时写给审核员看的说明（作者侧看不到这句）。
//
// 作者侧只看到枚举对应的固定文案（"内容不合规"），模型给出的类别与依据
// 一律不出现在任何面向作者的响应里（ADR-0024 决策 7）。
const AutoRejectNote = "系统自动拒绝：合规判定命中，请人工复核"

// AutoRejectForCompliance 把一篇文章按合规判定结果自动拒绝（票 313 / ADR-0025）。
//
// 落点与质量闸门**刻意不同**：质量不达标落草稿（作者改一改就可能通过），
// 合规命中落「已拒绝」并留一条待处理任务——前者是"还差几分"，后者是
// "这篇东西不该在这"，静静沉在草稿里没人会再看到它。
//
// 它在一个事务里做三件事：
//  1. 文章状态置为「已拒绝」（已发布或审核中都可，见 domain.RevokeForCompliance）；
//  2. 撤销原有的待处理任务，避免同一次提交留下两条待处理；
//  3. 新建一条**待处理**任务（source=compliance），让管理员能在列表里看到并复盘。
//
// 配额不在这里扣：提交时已经预占过一次，自动拒是异步副作用，再扣一次就是
// 一次提交扣两次（验收标准 3）。
func (s *Service) AutoRejectForCompliance(ctx context.Context, articleID domain.ArticleID) error {
	if articleID.IsZero() {
		return domain.ErrArticleNotFound
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		// 只有"审核中"与"已发布"需要撤下来：
		//  - 已拒绝：本次判定只补记录，不重复建任务（创建与发布各判一次的情形）；
		//  - 草稿：还没出去过，作者自己改就行——自动拒在这里会把一篇能改好的
		//    稿子变成"已拒绝"，反而更难恢复。
		switch a.Status() {
		case domain.StatusExamining, domain.StatusPublished:
		default:
			return nil
		}

		now := s.clock.Now()
		if err := a.RevokeForCompliance(now); err != nil {
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}

		if err := cancelPendingReviewTasks(ctx, r, articleID, "合规自动拒绝，任务作废", now); err != nil {
			return err
		}
		if err := createReviewTask(ctx, s, r, a, review.Source(article_review_enum.SourceCompliance), now); err != nil {
			return err
		}

		return emit(ctx, r, EventArticleRejected, articleID)
	})
}

// ComplianceRejectionLabel 返回自动拒时给作者看的固定文案。
//
// 它只依赖枚举，不读模型结论：作者永远看不到类别与依据。
func ComplianceRejectionLabel() string {
	return review.ReasonLabel(article_review_enum.ReasonCompliance)
}
