package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// 票 302 新规则：提交时同步风险扫描命中 medium 及以上 → 直接 REJECTED，
// 不给改；作者只拿到固定文案，命中词不外泄。
func TestCreateMediumRiskIsRejected(t *testing.T) {
	t.Parallel()

	// 观察级词命中 = medium；阻断级词命中 = high，两者都该拒。
	for _, tc := range []struct {
		name    string
		lists   moderation.WordLists
		content string
	}{
		{"观察级词命中", moderation.WordLists{Watch: []string{"加微信"}}, "欢迎加微信"},
		{"阻断级词命中", moderation.WordLists{Block: []string{"代开发票"}}, "代开发票"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repos := newModerationService(t, nil, func() moderation.WordLists { return tc.lists })
			ctx := context.Background()

			a, err := svc.Create(ctx, app.CreateArticleCmd{
				Actor:         authorViewer(),
				Policy:        domain.OpenWritePolicy(),
				Title:         "投稿",
				Content:       tc.content,
				TargetStatus:  domain.StatusExamining,
				SkipExamining: false,
			})
			if err == nil {
				t.Fatal("命中 medium 及以上风险应直接拒绝提交")
			}
			if !apperr.IsCode(err, apperr.CodeArticleRiskRejected) {
				t.Fatalf("错误码应为 article.risk_rejected，实际 %v", err)
			}
			// 业务拒绝是"带数据的拒绝"：文章仍以 REJECTED 落库，不给改。
			if a != nil {
				t.Fatal("拒绝时不应返回文章实体")
			}
			all, _, err := repos.FakeArticles().List(ctx, domain.Filter{}, domain.Page{Limit: 10})
			if err != nil || len(all) != 1 {
				t.Fatalf("文章应已落库且只有 1 篇: n=%d err=%v", len(all), err)
			}
			stored := all[0]
			if stored.Status() != domain.StatusRejected {
				t.Fatalf("应落 REJECTED，实际 %s", stored.Status())
			}
			if stored.IsPubliclyVisible() {
				t.Fatal("被拒文章不该对读者可见")
			}

			tasks, _ := repos.FakeReviews().ListPendingByArticle(ctx, stored.ID())
			if len(tasks) != 1 || tasks[0].Source() != review.Source(article_review_enum.SourceRiskCheck) {
				t.Fatalf("应留 1 条 risk_check 复盘任务，实际 %d 条", len(tasks))
			}
			if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticleRejected {
				t.Fatalf("直接拒应发 rejected 事件（触发通知），实际 %v", names)
			}
		})
	}
}

// low / none 不该拦人，仍按"已发布待可见"放行。
func TestCreateLowRiskStillPublishesPendingReview(t *testing.T) {
	t.Parallel()

	// 评分派生只给 low 的组合：总分略高但篇幅偏短等不构成 medium。
	// 这里用空词表 + 无评分记录，等价于 none。
	svc, _ := newModerationService(t, nil, nil)
	a, err := svc.Create(context.Background(), app.CreateArticleCmd{
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "干净稿",
		Content:       "正文",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: false,
	})
	if err != nil {
		t.Fatalf("none 风险不该拦人: %v", err)
	}
	if a.Status() != domain.StatusPublished || a.Visibility() != domain.VisibilityPendingReview {
		t.Fatalf("应落 Published + pending_review，实际 %s / %s", a.Status(), a.Visibility())
	}
}

// 站点免审核开启：全部直接真发布，同步风险扫描也不拦（运营紧急放行）。
func TestSkipExaminingBypassesRiskScan(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{Block: []string{"代开发票"}}
	svc, repos := newModerationService(t, nil, func() moderation.WordLists { return lists })
	a, err := svc.Create(context.Background(), app.CreateArticleCmd{
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "投稿",
		Content:       "代开发票",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: true,
	})
	if err != nil {
		t.Fatalf("免审核开启时不应被风险扫描拦下: %v", err)
	}
	if a.Status() != domain.StatusPublished || a.Visibility() != domain.VisibilityVisible {
		t.Fatalf("免审核应真发布且可见，实际 %s / %s", a.Status(), a.Visibility())
	}
	tasks, _ := repos.FakeReviews().ListPendingByArticle(context.Background(), a.ID())
	if len(tasks) != 0 {
		t.Fatalf("免审核直发不该建任务，实际 %d 条", len(tasks))
	}
}

// 异步分流：模型判 clean → 真正放开可见。
func TestPublishForComplianceMakesArticleVisible(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()
	a := apptest.NewArticle().
		ID(ctype.NewID(7)).Author(author).
		Status(domain.StatusPublished).
		Visibility(domain.VisibilityPendingReview).
		Build()
	repos.FakeArticles().Seed(a)

	if err := svc.PublishForCompliance(ctx, a.ID()); err != nil {
		t.Fatalf("放行失败: %v", err)
	}
	stored, _ := repos.FakeArticles().Find(ctx, a.ID())
	if stored.Visibility() != domain.VisibilityVisible || !stored.IsPubliclyVisible() {
		t.Fatalf("放行后应 visible 且对读者可见，实际 %s", stored.Visibility())
	}
	// 幂等：重复投递不报错。
	if err := svc.PublishForCompliance(ctx, a.ID()); err != nil {
		t.Fatalf("幂等重投不该报错: %v", err)
	}
}

// 额度延迟重审（ADR-0038）：改回审核中，但**不建**人工任务（次日自动重审）。
func TestDeferForComplianceCreatesNoManualTask(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()
	a := apptest.NewArticle().
		ID(ctype.NewID(9)).Author(author).
		Status(domain.StatusPublished).
		Visibility(domain.VisibilityPendingReview).
		Build()
	repos.FakeArticles().Seed(a)

	if err := svc.DeferForCompliance(ctx, a.ID()); err != nil {
		t.Fatalf("延迟重审转态失败: %v", err)
	}
	stored, _ := repos.FakeArticles().Find(ctx, a.ID())
	if stored.Status() != domain.StatusExamining {
		t.Fatalf("应改回审核中，实际 %s", stored.Status())
	}
	if stored.IsPubliclyVisible() {
		t.Fatal("审核中不该对读者可见")
	}
	// 关键差异：这里不建待处理任务，管理员队列不该多出一行"明天就自动过"的任务。
	tasks, _ := repos.FakeReviews().ListPendingByArticle(ctx, a.ID())
	if len(tasks) != 0 {
		t.Fatalf("延迟重审不该建人工任务，实际 %d 条", len(tasks))
	}
	// 幂等：文章已不在 pending_review，重复投递按"目的已达成"处理。
	if err := svc.DeferForCompliance(ctx, a.ID()); err != nil {
		t.Fatalf("幂等重投不该报错: %v", err)
	}
}

// 异步分流：模型判 suspect / 判定失败 → 改回审核中并建人工任务。
func TestBackToReviewForCompliance(t *testing.T) {
	t.Parallel()

	for _, src := range []article_review_enum.Source{
		article_review_enum.SourceModelSuspect,
		article_review_enum.SourceModelFailure,
	} {
		t.Run(string(src), func(t *testing.T) {
			svc, repos := newModerationService(t, nil, nil)
			ctx := context.Background()
			a := apptest.NewArticle().
				ID(ctype.NewID(7)).Author(author).
				Status(domain.StatusPublished).
				Visibility(domain.VisibilityPendingReview).
				Build()
			repos.FakeArticles().Seed(a)

			if err := svc.BackToReviewForCompliance(ctx, a.ID(), src); err != nil {
				t.Fatalf("转人工失败: %v", err)
			}
			stored, _ := repos.FakeArticles().Find(ctx, a.ID())
			if stored.Status() != domain.StatusExamining {
				t.Fatalf("应改回审核中，实际 %s", stored.Status())
			}
			if stored.IsPubliclyVisible() {
				t.Fatal("审核中不该对读者可见")
			}
			tasks, _ := repos.FakeReviews().ListPendingByArticle(ctx, a.ID())
			if len(tasks) != 1 || tasks[0].Source() != review.Source(src) {
				t.Fatalf("应建 1 条 %s 来源任务，实际 %d 条", src, len(tasks))
			}
			// 幂等：文章已不在 pending_review，重复投递按"目的已达成"处理。
			if err := svc.BackToReviewForCompliance(ctx, a.ID(), src); err != nil {
				t.Fatalf("幂等重投不该报错: %v", err)
			}
		})
	}
}

// 审核通过时清掉 pending_review：不留"已发布但读者看不到"的中间态。
func TestApproveClearsPendingReview(t *testing.T) {
	t.Parallel()

	a := apptest.NewArticle().
		ID(ctype.NewID(9)).Author(author).
		Status(domain.StatusExamining).
		Visibility(domain.VisibilityPendingReview).
		Build()
	if err := a.Approve(adminViewer().UserID, apptest.DefaultTime); err != nil {
		t.Fatalf("审核通过失败: %v", err)
	}
	if a.Status() != domain.StatusPublished || a.Visibility() != domain.VisibilityVisible {
		t.Fatalf("审核通过后应真发布且可见，实际 %s / %s", a.Status(), a.Visibility())
	}
}

// 作者不能通过"编辑文章"接口把 pending_review 改成 visible 绕过模型判定。
// 这是 Spec 审查发现的真实旁路：CanSetVisibility 只管管理员改可见性端点，
// 通用 Update 走的是 ApplyEdit，必须自己再挡一次。
func TestApplyEditCannotPublishPendingReview(t *testing.T) {
	t.Parallel()

	a := apptest.NewArticle().
		ID(ctype.NewID(11)).Author(author).
		Status(domain.StatusPublished).
		Visibility(domain.VisibilityPendingReview).
		Build()
	next := domain.VisibilityVisible
	flags, err := a.ApplyEdit(domain.EditPatch{Visibility: &next}, apptest.DefaultTime)
	if err != nil {
		t.Fatalf("编辑不该报错: %v", err)
	}
	if a.Visibility() != domain.VisibilityPendingReview {
		t.Fatalf("pending_review 不该被编辑成 %s", a.Visibility())
	}
	if flags.Visibility {
		t.Fatal("无效改动不该报 Visibility 变更，否则会误导调用方以为已改")
	}
	if a.IsPubliclyVisible() {
		t.Fatal("绕过检查：文章仍不该对读者可见")
	}
}

// 系统直接拒（risk_check / compliance）是终局，不能靠"编辑 + 提交"重投；
// 人工驳回的 REJECTED 不在此列，#303 仍允许改稿重投。
func TestSystemRejectedCannotResubmit(t *testing.T) {
	t.Parallel()

	for _, src := range []article_review_enum.Source{
		article_review_enum.SourceRiskCheck,
		article_review_enum.SourceCompliance,
	} {
		t.Run(string(src), func(t *testing.T) {
			svc, repos := newModerationService(t, nil, nil)
			ctx := context.Background()
			id := ctype.NewID(21)
			a := apptest.NewArticle().ID(id).Author(author).
				Status(domain.StatusRejected).Visibility(domain.VisibilityVisible).Build()
			repos.FakeArticles().Seed(a)
			repos.FakeReviews().Seed(apptest.NewTaskWithSourceForTest(
				22, id, author, review.StatusPending, review.Source(src)))

			content := "改过的正文"
			_, err := svc.Update(ctx, app.UpdateArticleCmd{
				Actor:         authorViewer(),
				Policy:        domain.OpenWritePolicy(),
				ArticleID:     id,
				Content:       &content,
				TargetStatus:  statusPtr(domain.StatusExamining),
				SkipExamining: false,
			})
			if err == nil {
				t.Fatal("系统直接拒之后不该能再提交")
			}
			if !apperr.IsCode(err, apperr.CodeArticleInvalidState) {
				t.Fatalf("错误码应为 article.invalid_state，实际 %v", err)
			}
		})
	}
}

func statusPtr(s domain.Status) *domain.Status { return &s }

// 作者不能自己把 pending_review 改成 visible 绕过模型判定。
func TestAuthorCannotPublishPendingReviewByHand(t *testing.T) {
	t.Parallel()

	a := apptest.NewArticle().
		ID(ctype.NewID(11)).Author(author).
		Status(domain.StatusPublished).
		Visibility(domain.VisibilityPendingReview).
		Build()
	if domain.CanSetVisibility(a, authorViewer(), domain.VisibilityVisible) {
		t.Fatal("作者不该能提前放行 pending_review")
	}
	if domain.CanSetVisibility(a, adminViewer(), domain.VisibilityVisible) {
		t.Fatal("管理员也不该用改可见性接口绕过模型判定（应走审核通过）")
	}
}
