package domain_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 本文件锁定「管理员强制退回草稿」这条转移的起点集合与落点。
//
// 它和 DemoteForQuality 的差别正是这些用例的意义：后者只认「已发布待可见」，
// 而运营纠错面对的文章**已经对读者可见**（线上漏放的那篇 6 分文章就是这样），
// 所以这里把四种可见性都跑一遍。

// publishedWith 构造一篇"已发布 + 指定可见性"的文章。
func publishedWith(t *testing.T, visibility domain.Visibility) *domain.Article {
	t.Helper()
	submitted := baseTime.Add(-2 * time.Hour)
	reviewed := baseTime.Add(-time.Hour)
	reviewer := domain.UserID(ctype.NewID(900))
	return domain.Restore(domain.RestoreParams{
		ID:          ctype.NewID(7001),
		AuthorID:    authorID,
		Title:       "被错误放行的文章",
		Content:     "正文",
		Status:      domain.StatusPublished,
		Visibility:  visibility,
		SubmittedAt: &submitted,
		ReviewedAt:  &reviewed,
		ReviewedBy:  &reviewer,
		CreatedAt:   baseTime.Add(-3 * time.Hour),
		UpdatedAt:   baseTime.Add(-time.Hour),
	})
}

// TestForceDemoteFromPublishedAnyVisibility 钉住"任何可见性的已发布文章都能退回"。
//
// 这正是 DemoteForQuality 做不到的那一档：它要求 visibility == pending_review，
// 对已经 visible 的文章返回 ErrInvalidTransition。运营要的恰恰是把已可见的撤下来。
func TestForceDemoteFromPublishedAnyVisibility(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		visibility domain.Visibility
	}{
		{name: "已公开可见", visibility: domain.VisibilityVisible},
		{name: "已发布待可见", visibility: domain.VisibilityPendingReview},
		{name: "作者隐藏", visibility: domain.VisibilityUserHidden},
		{name: "管理员隐藏", visibility: domain.VisibilityAdminHidden},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := publishedWith(t, tc.visibility)
			now := baseTime.Add(time.Hour)
			if err := a.ForceDemote(now); err != nil {
				t.Fatalf("强制退回不应失败: %v", err)
			}
			if a.Status() != domain.StatusDraft {
				t.Fatalf("状态应为草稿，实际 %s", a.Status())
			}
			if a.Visibility() != domain.VisibilityVisible {
				t.Fatalf("可见性应复位成 visible，实际 %s", a.Visibility())
			}
			if a.IsPubliclyVisible() {
				t.Fatal("退回后不该再对匿名访客可见")
			}
			// 审核痕迹必须清空：留着它们会让后台显示"这篇文章刚被人审过"，
			// 而实际上这次是一次撤回，下一次提交要重新走一遍判定。
			if a.SubmittedAt() != nil || a.ReviewedAt() != nil || a.ReviewedBy() != nil {
				t.Fatalf("审核痕迹应清空: submitted=%v reviewed=%v by=%v",
					a.SubmittedAt(), a.ReviewedAt(), a.ReviewedBy())
			}
			if !a.UpdatedAt().Equal(now) {
				t.Fatalf("updatedAt 应为传入时间，实际 %v", a.UpdatedAt())
			}
		})
	}
}

// TestForceDemoteFromExaminingAndRejected 钉住另外两个起点：审核中（还没放行但
// 已在发布链路上）与已拒绝（结论可能是误判，需要把文章交回作者）。
func TestForceDemoteFromExaminingAndRejected(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.Status{domain.StatusExamining, domain.StatusRejected} {
		status := status
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()

			a := domain.Restore(domain.RestoreParams{
				ID:       ctype.NewID(7002),
				AuthorID: authorID,
				Title:    "标题",
				Content:  "正文",
				Status:   status,
			})
			if err := a.ForceDemote(baseTime.Add(time.Hour)); err != nil {
				t.Fatalf("%s 应能退回草稿: %v", status, err)
			}
			if a.Status() != domain.StatusDraft {
				t.Fatalf("状态应为草稿，实际 %s", a.Status())
			}
		})
	}
}

// TestForceDemoteFromDraftIsInvalid 钉住"已是草稿"的语义：domain 报错，
// 由用例层按"目的已达成"翻译成成功（幂等）。
func TestForceDemoteFromDraftIsInvalid(t *testing.T) {
	t.Parallel()

	a := newDraft(t)
	err := a.ForceDemote(baseTime.Add(time.Hour))
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("已是草稿应返回 ErrInvalidTransition，实际 %v", err)
	}
	if a.Status() != domain.StatusDraft {
		t.Fatalf("失败时不该改变状态，实际 %s", a.Status())
	}
}

// TestForceDemoteDeletedArticleIsNotFound 钉住软删：退回一篇已删除的文章必须报
// "不存在"，而不是把它悄悄改回草稿（那会让管理员的列表里凭空多出一篇草稿）。
func TestForceDemoteDeletedArticleIsNotFound(t *testing.T) {
	t.Parallel()

	a := domain.Restore(domain.RestoreParams{
		ID:       ctype.NewID(7003),
		AuthorID: authorID,
		Title:    "已删除",
		Content:  "正文",
		Status:   domain.StatusPublished,
		Deleted:  true,
	})
	if err := a.ForceDemote(baseTime.Add(time.Hour)); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("已删除的文章应返回 ErrArticleNotFound，实际 %v", err)
	}
}
