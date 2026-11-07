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

func newModerationService(t *testing.T, scores app.ModerationScorePort, words app.ModerationWordProvider) (*app.Service, *apptest.Repos) {
	t.Helper()
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), nil, scores, words)
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

// 第①②层合并进审核列表：词表命中与评分派生标签都出现在条目上，
// 等级取最严重者（票 311 验收标准 1、3、4）。
func TestListReviewTasksMarksRiskFromWordsAndScore(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().Seed(ctype.NewID(1), moderation.ScoreFacts{
		TotalScore:     45,
		DimensionsJSON: `[{"name":"clarity","score":10},{"name":"structure","score":70}]`,
		ContentLength:  120,
	})
	lists := moderation.WordLists{Block: []string{"代开发票"}, Watch: []string{"加微信"}}
	svc, repos := newModerationService(t, scores, func() moderation.WordLists { return lists })
	seedReviewedArticle(t, repos, 1, 10, "代开发票请加微信")

	views, _, err := svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortCreatedAt, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("列出审核任务失败: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("应有 1 条任务，实际 %d", len(views))
	}
	if views[0].Risk.Level != moderation.LevelHigh {
		t.Fatalf("阻断级命中应把风险抬到 high，实际 %s", views[0].Risk.Level)
	}

	want := []string{"block_word", "watch_word", "low_score", "dimension_collapse", "length_abnormal"}
	got := views[0].Risk.Labels()
	if len(got) != len(want) {
		t.Fatalf("风险标签应为 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("风险标签应为 %v，实际 %v", want, got)
		}
	}
}

// 没有评分记录的文章照常出现在列表里，风险只按词表层算（票 311 验收标准 5）。
func TestListReviewTasksWithoutScoreRecordIsNotAnError(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, newFakeScores(), nil)
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
		t.Fatalf("无评分、无词表命中应为 none，实际 %s", views[0].Risk.Level)
	}
	if len(views[0].Risk.Labels()) != 0 {
		t.Fatalf("不应有风险标签，实际 %v", views[0].Risk.Labels())
	}
}

// 词表是运行期数据、结论实时重算：改词表后下一次读取立即不同，不落库、不缓存（票 311 验收标准 2）。
func TestListReviewTasksRecomputesAfterWordListChange(t *testing.T) {
	t.Parallel()

	scores := newFakeScores()
	lists := moderation.WordLists{}
	svc, repos := newModerationService(t, scores, func() moderation.WordLists { return lists })
	seedReviewedArticle(t, repos, 3, 30, "这篇稿子里有代开发票")

	ctx := context.Background()
	page := domain.Page{Limit: 10}

	views, _, err := svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, page)
	if err != nil {
		t.Fatalf("第一次读取失败: %v", err)
	}
	if views[0].Risk.Level != moderation.LevelNone {
		t.Fatalf("词表为空时不应有风险，实际 %s", views[0].Risk.Level)
	}

	lists = moderation.WordLists{Block: []string{"代开发票"}}
	views, _, err = svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, page)
	if err != nil {
		t.Fatalf("改词表后读取失败: %v", err)
	}
	if views[0].Risk.Level != moderation.LevelHigh {
		t.Fatalf("改词表后应立即按新词表判为 high，实际 %s", views[0].Risk.Level)
	}
	if labels := views[0].Risk.Labels(); len(labels) != 1 || labels[0] != "block_word" {
		t.Fatalf("应只剩 block_word 标签，实际 %v", labels)
	}
}

// 按风险排序：高风险在前，同等级保持提交时间升序（老投稿不因同等级被插队）。
func TestListReviewTasksSortsByRiskLevel(t *testing.T) {
	t.Parallel()

	scores := newFakeScores().
		Seed(ctype.NewID(41), moderation.ScoreFacts{TotalScore: 80, ContentLength: 1000}).
		Seed(ctype.NewID(42), moderation.ScoreFacts{TotalScore: 40, ContentLength: 1000})
	lists := moderation.WordLists{Block: []string{"代开发票"}, Watch: []string{"加微信"}}
	svc, repos := newModerationService(t, scores, func() moderation.WordLists { return lists })

	base := apptest.DefaultTime
	// 提交时间升序：41（低风险/无命中）→ 42（低分 medium）→ 43（观察级 medium）→ 44（阻断级 high）。
	for i, spec := range []struct {
		articleID uint64
		content   string
		at        time.Time
	}{
		{41, "普通正文", base.Add(0 * time.Minute)},
		{42, "普通正文", base.Add(1 * time.Minute)},
		{43, "欢迎加微信", base.Add(2 * time.Minute)},
		{44, "代开发票", base.Add(3 * time.Minute)},
	} {
		a := apptest.NewArticle().
			ID(ctype.NewID(spec.articleID)).
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
	if got[0] != "high" || got[1] == "none" || got[2] == "none" || got[3] != "none" {
		t.Fatalf("风险排序结果不是 high → 非 none → 非 none → none：%v", got)
	}
	// 两条 medium 里，提交更早的 43 在前。
	if views[1].Task.ArticleID() != ctype.NewID(42) || views[2].Task.ArticleID() != ctype.NewID(43) {
		t.Fatalf("同等级应按提交时间升序，实际 medium 顺序为 %s、%s",
			views[1].Task.ArticleID(), views[2].Task.ArticleID())
	}
	if views[3].Task.ArticleID() != ctype.NewID(41) {
		t.Fatalf("无风险条目应排最后，实际 %s", views[3].Task.ArticleID())
	}

	// 分页在排序之后切：第一页取最高风险的两条。
	views, total, err = svc.ListReviewTasks(
		context.Background(), adminViewer(), nil, app.ReviewSortRisk, domain.Page{Offset: 0, Limit: 2})
	if err != nil {
		t.Fatalf("按风险排序分页失败: %v", err)
	}
	if total != 4 || len(views) != 2 {
		t.Fatalf("分页应保留总数 4 并返回 2 条，实际 total=%d len=%d", total, len(views))
	}
	if views[0].Task.ArticleID() != ctype.NewID(44) || views[1].Task.ArticleID() != ctype.NewID(42) {
		t.Fatalf("第一页应是 44、42，实际 %s、%s",
			views[0].Task.ArticleID(), views[1].Task.ArticleID())
	}
}
