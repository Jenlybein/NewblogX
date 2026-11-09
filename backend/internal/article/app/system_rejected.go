package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// 系统直接拒是终局（票 302 / ADR-0035「REJECTED，不给改」）。
//
// 人工驳回的 StatusRejected 允许作者改稿重投（#303），所以要按**复盘任务的
// 来源**区分，而不是只认文章状态：同步风险命中留 risk_check、合规自动拒留
// compliance，两者都不许再提交。管理员若要放行，走后台把任务处理掉再改状态，
// 不由作者用"编辑 + 提交"绕过。

// CodeArticleSystemRejected 没有单独的对外码：复用 article.invalid_state
// （状态机拒绝），作者侧拿到"当前状态不允许该操作"，不暴露任何判定细节。

// rejectSystemRejectedResubmit 拦下"系统直接拒之后再提交"。
func rejectSystemRejectedResubmit(ctx context.Context, r Repos, articleID domain.ArticleID) error {
	tasks, err := r.Reviews().ListPendingByArticle(ctx, articleID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		switch article_review_enum.Source(task.Source()) {
		case article_review_enum.SourceRiskCheck, article_review_enum.SourceCompliance:
			return apperr.New(apperr.CodeArticleInvalidState)
		}
	}
	return nil
}
