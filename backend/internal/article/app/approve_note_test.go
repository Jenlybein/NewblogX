package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
)

// testQualityGate 是一份固定的闸门配置：及格线 50 + 灰带 2 → 放行线 52。
//
// 用例测试关心的是"留痕用哪条线"，不关心配置怎么读：闸门快照由 HTTP 层
// 每请求现读后传进来（app 层不读配置），所以这里直接给一个写死的快照。
func testQualityGate() app.QualityGatePolicy {
	return app.QualityGatePolicy{PassScore: 50, GrayBandWidth: 2}
}

// TestHandleReviewTaskApproveRecordsScoreNote 钉住①：管理员放行时把
// "当前多少分、放行线多少"写进任务行的放行留痕，而不是静默通过。
//
// 管理员通过**豁免质量闸门**（终审），但豁免不等于没发生——这句留痕就是
// 事后复盘"这次是越级放行"的唯一依据。
func TestHandleReviewTaskApproveRecordsScoreNote(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().Seed(ctype.NewID(1), moderation.ScoreFacts{TotalScore: 47})
	svc, repos := newModerationService(t, scores)
	ctx := context.Background()
	seedReviewedArticle(t, repos, 1, 11, "正文甲")

	if _, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(11), true, "", "", testQualityGate()); err != nil {
		t.Fatalf("管理员通过应成功，实际 %v", err)
	}

	task, err := repos.FakeReviews().Find(ctx, 11)
	if err != nil {
		t.Fatalf("查任务失败: %v", err)
	}
	if task.Status() != review.StatusApproved {
		t.Fatalf("任务应为已通过，实际 %s", task.Status())
	}
	// reason 仍是驳回原因列：通过不许借它的位置记东西。
	if task.Reason() != "" {
		t.Fatalf("通过不该在 reason 上留内容，实际 %q", task.Reason())
	}
	if want := "管理员以 47 分放行（放行线 52 分）"; task.ApproveNote() != want {
		t.Fatalf("放行留痕应为 %q，实际 %q", want, task.ApproveNote())
	}
	// 幂等：同一任务重复通过沿用既有状态判断（文章已不在审核中 → 拒绝，
	// 任务本身也会被判"已处理"），不会写出第二条留痕。
	_, err = svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(11), true, "", "", testQualityGate())
	if !errors.Is(err, domain.ErrInvalidTransition) && !errors.Is(err, review.ErrTaskHandled) {
		t.Fatalf("重复通过应被既有状态判断挡住，实际 %v", err)
	}
	again, _ := repos.FakeReviews().Find(ctx, 11)
	if again.ApproveNote() != "管理员以 47 分放行（放行线 52 分）" {
		t.Fatalf("重复通过不该改写/追加留痕，实际 %q", again.ApproveNote())
	}
}

// TestHandleReviewTaskApproveWithoutScoreStillReleases 钉住②：取不到分数时
// 管理员**仍能放行**，且留痕写明"未知分数"——不报错，也不假装是 0 分。
func TestHandleReviewTaskApproveWithoutScoreStillReleases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scores app.ModerationScorePort
	}{
		// AI 关闭 / 端口没装配：审核列表照样能用（与列表同一条口径）。
		{"评分端口缺失", nil},
		// 端口在、文章还没评分：同样取不到分数。
		{"文章没有评分记录", newFakeScores()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos := newModerationService(t, c.scores)
			ctx := context.Background()
			seedReviewedArticle(t, repos, 1, 11, "正文甲")

			if _, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(11), true, "", "", testQualityGate()); err != nil {
				t.Fatalf("取不到分数不该挡住管理员放行，实际 %v", err)
			}

			task, _ := repos.FakeReviews().Find(ctx, 11)
			if want := "管理员以未知分数放行（放行线 52 分）"; task.ApproveNote() != want {
				t.Fatalf("无分数时留痕应为 %q，实际 %q", want, task.ApproveNote())
			}
			// 放行确实生效：文章已发布、任务已通过。
			a, _ := repos.FakeArticles().Find(ctx, ctype.NewID(1))
			if a.Status() != domain.StatusPublished {
				t.Fatalf("管理员应已放行，文章实际状态 %s", a.Status())
			}
			if task.Status() != review.StatusApproved {
				t.Fatalf("任务应为已通过，实际 %s", task.Status())
			}
		})
	}
}

// TestListReviewTasksExposesCurrentScoreAndPassLine 钉住审核队列把
// "当前 X 分（放行线 Y 分）"带给前端：有评分给指针（0 分也给），
// 没评分保持 nil——nil 与 0 分不是一回事。
func TestListReviewTasksExposesCurrentScoreAndPassLine(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().Seed(ctype.NewID(1), moderation.ScoreFacts{TotalScore: 47})
	svc, repos := newModerationService(t, scores)
	ctx := context.Background()
	seedReviewedArticle(t, repos, 1, 11, "正文甲")
	// 第二篇有任务但没有评分记录：分数应是 nil，而不是 0。
	a := apptest.NewArticle().
		ID(ctype.NewID(2)).
		Author(author).
		Status(domain.StatusExamining).
		Title("没评分的稿子").
		Content("正文乙").
		Build()
	repos.FakeArticles().Seed(a)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(12, a.ID(), author, review.StatusPending))

	views, _, err := svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, testQualityGate(), domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("列出审核任务失败: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("应有 2 条任务，实际 %d", len(views))
	}
	if views[0].QualityPassScore != 52 || views[1].QualityPassScore != 52 {
		t.Fatalf("放行线应为 52，实际 %d / %d", views[0].QualityPassScore, views[1].QualityPassScore)
	}
	if views[0].QualityScore == nil || *views[0].QualityScore != 47 {
		t.Fatalf("有评分时总分应为 47，实际 %v", views[0].QualityScore)
	}
	if views[1].QualityScore != nil {
		t.Fatalf("没有评分记录时分数应为 nil（不是 0），实际 %v", *views[1].QualityScore)
	}
}
