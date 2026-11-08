package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
)

// seedPublishedArticle 预置一篇**已发布**文章（默认站点跳过审核，投稿即发布，
// 合规判定在发布之后才到）。
func seedPublishedArticle(t *testing.T, repos *apptest.Repos, articleID uint64) domain.ArticleID {
	t.Helper()
	a := apptest.NewArticle().
		ID(ctype.NewID(articleID)).
		Author(author).
		Status(domain.StatusPublished).
		Title("已发布稿").
		Content("正文").
		Build()
	repos.FakeArticles().Seed(a)
	return a.ID()
}

// 已发布文章被合规判定命中：状态置为已拒绝，并留下一条待处理任务（票 313）。
func TestAutoRejectTurnsPublishedIntoRejectedWithPendingTask(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()

	id := seedPublishedArticle(t, repos, 1)

	if err := svc.AutoRejectForCompliance(ctx, id); err != nil {
		t.Fatalf("自动拒失败: %v", err)
	}

	got, err := repos.FakeArticles().Find(ctx, id)
	if err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	if got.Status() != domain.StatusRejected {
		t.Fatalf("已发布文章应被置为已拒绝，实际 %v", got.Status())
	}

	// 留下可查的提交痕迹：一条待处理任务，来源是合规。
	tasks, _, err := repos.FakeReviews().List(ctx, nil, 0, 100)
	if err != nil {
		t.Fatalf("读审核任务失败: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.ArticleID() == id && task.Status() == review.StatusPending {
			found = true
			if task.Source() != review.Source("compliance") {
				t.Fatalf("自动拒的任务来源应为 compliance，实际 %s", task.Source())
			}
		}
	}
	if !found {
		t.Fatal("自动拒应留下一条待处理任务供管理员复盘")
	}
}

// 草稿不自动拒：还没出去过，作者自己改就行。
func TestAutoRejectSkipsDraft(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()

	draft := apptest.NewArticle().
		ID(ctype.NewID(2)).
		Author(author).
		Status(domain.StatusDraft).
		Title("草稿").
		Content("正文").
		Build()
	repos.FakeArticles().Seed(draft)

	if err := svc.AutoRejectForCompliance(ctx, draft.ID()); err != nil {
		t.Fatalf("草稿上自动拒不该报错（应为空操作）: %v", err)
	}
	got, err := repos.FakeArticles().Find(ctx, draft.ID())
	if err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	if got.Status() != domain.StatusDraft {
		t.Fatalf("草稿应保持草稿，实际 %v", got.Status())
	}
}

// 已拒绝文章不重复建任务（创建与发布各判一次的情形）。
func TestAutoRejectIsIdempotentForRejectedArticle(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()

	id := seedPublishedArticle(t, repos, 3)

	if err := svc.AutoRejectForCompliance(ctx, id); err != nil {
		t.Fatalf("首次自动拒失败: %v", err)
	}
	before, _, _ := repos.FakeReviews().List(ctx, nil, 0, 100)
	if err := svc.AutoRejectForCompliance(ctx, id); err != nil {
		t.Fatalf("重复自动拒不该报错: %v", err)
	}
	after, _, _ := repos.FakeReviews().List(ctx, nil, 0, 100)
	if len(after) != len(before) {
		t.Fatalf("重复判定不该再建任务（%d → %d）", len(before), len(after))
	}
}

// 自动拒给作者看的文案只来自枚举，不含模型结论。
func TestComplianceRejectionLabelIsGeneric(t *testing.T) {
	t.Parallel()

	label := app.ComplianceRejectionLabel()
	if label == "" {
		t.Fatal("自动拒必须有可展示的文案")
	}
	if label == app.AutoRejectNote {
		t.Fatal("作者看到的文案不能是写给审核员看的那句说明")
	}
}
