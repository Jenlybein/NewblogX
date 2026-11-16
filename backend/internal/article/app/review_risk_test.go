package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
)

// fakeScores 是 app.ModerationScorePort 的内存实现：没有记录的文章不出现在 map 里。
type fakeScores struct {
	byArticle map[domain.ArticleID]moderation.ScoreFacts
}

func newFakeScores() fakeScores {
	return fakeScores{byArticle: make(map[domain.ArticleID]moderation.ScoreFacts)}
}

func (f fakeScores) Seed(articleID domain.ArticleID, facts moderation.ScoreFacts) fakeScores {
	f.byArticle[articleID] = facts
	return f
}

func (f fakeScores) LatestScores(_ context.Context, ids []domain.ArticleID) (map[domain.ArticleID]moderation.ScoreFacts, error) {
	out := make(map[domain.ArticleID]moderation.ScoreFacts, len(ids))
	for _, id := range ids {
		if facts, ok := f.byArticle[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

func newModerationService(t *testing.T, scores app.ModerationScorePort) (*app.Service, *apptest.Repos) {
	t.Helper()
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), nil, scores, nil)
	repos.FakeUsers().Seed(app.AuthorView{ID: author, Nickname: "作者", Username: "u100"})
	repos.FakeCategories().Seed()
	return svc, repos
}

// seedReviewedArticle 预置一篇文章与一条待处理审核任务。
func seedReviewedArticle(t *testing.T, repos *apptest.Repos, articleID uint64, taskID uint64, content string) *domain.Article {
	t.Helper()
	a := apptest.NewArticle().
		ID(ctype.NewID(articleID)).
		Author(author).
		Status(domain.StatusExamining).
		Title("待审稿").
		Content(content).
		Build()
	repos.FakeArticles().Seed(a)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(taskID, a.ID(), author, review.StatusPending))
	return a
}

// 审核列表的结论只剩评分派生层：低分 / 维度塌陷 / 篇幅异常都在条目上，
// 等级取最严重者；词表信号已随「文章词表机审退场」删除（ADR-0032 决策 6）。
func TestListReviewTasksMarksRiskFromScore(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().Seed(ctype.NewID(1), moderation.ScoreFacts{
		TotalScore:     45,
		DimensionsJSON: `[{"name":"clarity","score":10},{"name":"structure","score":70}]`,
		ContentLength:  120,
	})
	svc, repos := newModerationService(t, scores)
	seedReviewedArticle(t, repos, 1, 10, "一段普通正文")

	views, _, err := svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortCreatedAt, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("列出审核任务失败: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("应有 1 条任务，实际 %d", len(views))
	}
	if views[0].Risk.Level != moderation.LevelMedium {
		t.Fatalf("45 分低于及格线应为 medium，实际 %s", views[0].Risk.Level)
	}

	want := []string{"low_score", "dimension_collapse", "length_abnormal"}
	got := views[0].Risk.Labels()
	if len(got) != len(want) {
		t.Fatalf("风险标签应为 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("风险标签应为 %v，实际 %v", want, got)
		}
	}
	for _, label := range got {
		if label == "block_word" || label == "watch_word" {
			t.Fatalf("审核列表不再应有词表信号，实际 %v", got)
		}
	}
}

// 没有评分记录的文章照常出现在列表里，风险是 none（票 311 验收标准 5）。
func TestListReviewTasksWithoutScoreRecordIsNotAnError(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, newFakeScores())
	seedReviewedArticle(t, repos, 2, 20, "一段普通正文")

	views, total, err := svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortCreatedAt, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("没有评分记录不应让队列失败: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("应有 1 条任务，实际 total=%d len=%d", total, len(views))
	}
	if views[0].Risk.Level != moderation.LevelNone {
		t.Fatalf("无评分应为 none，实际 %s", views[0].Risk.Level)
	}
	if len(views[0].Risk.Labels()) != 0 {
		t.Fatalf("不应有风险标签，实际 %v", views[0].Risk.Labels())
	}
}

// 评分是运行期数据、结论实时重算：新评分产生后下一次读取立即不同，不落库、不缓存（票 311 验收标准 2）。
func TestListReviewTasksRecomputesAfterNewScore(t *testing.T) {
	t.Parallel()

	scores := newFakeScores()
	svc, repos := newModerationService(t, scores)
	seedReviewedArticle(t, repos, 3, 30, "这篇稿子篇幅很短")

	ctx := context.Background()
	page := domain.Page{Limit: 10}

	views, _, err := svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, page)
	if err != nil {
		t.Fatalf("第一次读取失败: %v", err)
	}
	if views[0].Risk.Level != moderation.LevelNone {
		t.Fatalf("没有评分时不应有风险，实际 %s", views[0].Risk.Level)
	}

	// 同一份 map 被写入新评分：服务每次现读端口，所以下一次列表就应该看到。
	scores.Seed(ctype.NewID(3), moderation.ScoreFacts{TotalScore: 40, ContentLength: 1000})
	views, _, err = svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, page)
	if err != nil {
		t.Fatalf("写入新评分后读取失败: %v", err)
	}
	if views[0].Risk.Level != moderation.LevelMedium {
		t.Fatalf("低分应立即判为 medium，实际 %s", views[0].Risk.Level)
	}
	if labels := views[0].Risk.Labels(); len(labels) != 1 || labels[0] != "low_score" {
		t.Fatalf("应只剩 low_score 标签，实际 %v", labels)
	}
}

// 按风险排序：高风险在前，同等级保持提交时间升序（老投稿不因同等级被插队）。
func TestListReviewTasksSortsByRiskLevel(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().
		Seed(ctype.NewID(41), moderation.ScoreFacts{TotalScore: 80, ContentLength: 1000}).
		Seed(ctype.NewID(42), moderation.ScoreFacts{TotalScore: 40, ContentLength: 1000}).
		Seed(ctype.NewID(43), moderation.ScoreFacts{
			TotalScore:     80,
			DimensionsJSON: `[{"name":"clarity","score":10}]`,
			ContentLength:  1000,
		}).
		Seed(ctype.NewID(44), moderation.ScoreFacts{TotalScore: 80, ContentLength: 100})
	svc, repos := newModerationService(t, scores)

	base := apptest.DefaultTime
	// 提交时间升序：41（无风险）→ 42（低分 medium）→ 43（维度塌陷 low）→ 44（篇幅异常 low）。
	for i, spec := range []struct {
		// ID 用 ctype.ID 而不是裸 uint64：R12 对测试文件同样生效
		// （雪花 ID 超出 JS 安全整数，裸整型会让契约印成 integer）。
		articleID ctype.ID
		content   string
		at        time.Time
	}{
		{ctype.NewID(41), "普通正文", base.Add(0 * time.Minute)},
		{ctype.NewID(42), "普通正文", base.Add(1 * time.Minute)},
		{ctype.NewID(43), "普通正文", base.Add(2 * time.Minute)},
		{ctype.NewID(44), "普通正文", base.Add(3 * time.Minute)},
	} {
		a := apptest.NewArticle().
			ID(spec.articleID).
			Author(author).
			Status(domain.StatusExamining).
			Title("待审稿").
			Content(spec.content).
			Build()
		repos.FakeArticles().Seed(a)
		repos.FakeReviews().Seed(review.RestoreTask(review.RestoreTaskParams{
			ID:        ctype.NewID(uint64(100 + i)),
			ArticleID: a.ID(),
			AuthorID:  author,
			Stage:     review.StageManual,
			Source:    review.SourceCreate,
			Status:    review.StatusPending,
			CreatedAt: spec.at,
		}))
	}

	views, total, err := svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortRisk, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("按风险排序失败: %v", err)
	}
	if total != 4 {
		t.Fatalf("总数应为 4，实际 %d", total)
	}
	got := []string{
		string(views[0].Risk.Level),
		string(views[1].Risk.Level),
		string(views[2].Risk.Level),
		string(views[3].Risk.Level),
	}
	if got[0] != "medium" || got[1] != "low" || got[2] != "low" || got[3] != "none" {
		t.Fatalf("风险排序结果不是 medium → low → low → none：%v", got)
	}
	// 两条 low 里，提交更早的 43 在前；无风险的 41 排最后。
	if views[1].Task.ArticleID() != ctype.NewID(43) || views[2].Task.ArticleID() != ctype.NewID(44) {
		t.Fatalf("同等级应按提交时间升序，实际 low 顺序为 %s、%s",
			views[1].Task.ArticleID(), views[2].Task.ArticleID())
	}
	if views[0].Task.ArticleID() != ctype.NewID(42) {
		t.Fatalf("低分条目应排最前，实际 %s", views[0].Task.ArticleID())
	}
	if views[3].Task.ArticleID() != ctype.NewID(41) {
		t.Fatalf("无风险条目应排最后，实际 %s", views[3].Task.ArticleID())
	}

	// 分页在排序之后切：第一页取风险最高的两条。
	views, total, err = svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortRisk, domain.Page{Offset: 0, Limit: 2})
	if err != nil {
		t.Fatalf("按风险排序分页失败: %v", err)
	}
	if total != 4 || len(views) != 2 {
		t.Fatalf("分页应保留总数 4 并返回 2 条，实际 total=%d len=%d", total, len(views))
	}
	if views[0].Task.ArticleID() != ctype.NewID(42) || views[1].Task.ArticleID() != ctype.NewID(43) {
		t.Fatalf("第一页应是 42、43，实际 %s、%s",
			views[0].Task.ArticleID(), views[1].Task.ArticleID())
	}
}
