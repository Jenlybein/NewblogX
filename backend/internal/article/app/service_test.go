package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
)

const (
	author   = domain.UserID(100)
	stranger = domain.UserID(200)
	admin    = domain.UserID(900)
)

func newService(t *testing.T) (*app.Service, *apptest.Repos, *apptest.Tx) {
	t.Helper()
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000))
	repos.FakeUsers().Seed(app.AuthorView{ID: author, Nickname: "作者", Username: "u100"})
	repos.FakeCategories().Seed()
	return svc, repos, tx
}

// seedArticle 预置一篇指定状态的可见文章。
func seedArticle(t *testing.T, repos *apptest.Repos, id domain.ArticleID, authorID domain.UserID, status domain.Status) *domain.Article {
	t.Helper()
	a := apptest.NewArticle().ID(id).Author(authorID).Status(status).Build()
	repos.FakeArticles().Seed(a)
	return a
}

func authorViewer() domain.Viewer   { return domain.NormalUser(author) }
func strangerViewer() domain.Viewer { return domain.NormalUser(stranger) }
func adminViewer() domain.Viewer    { return domain.AdminUser(admin) }

// ── Create ──

func TestCreateDraft(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Title:        "第一篇",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
	})
	if err != nil {
		t.Fatalf("创建草稿失败: %v", err)
	}

	if a.Status() != domain.StatusDraft {
		t.Fatalf("应为草稿，实际 %s", a.Status())
	}
	if a.SubmittedAt() != nil {
		t.Fatal("草稿不应有提交时间")
	}
	if tx.Commits() != 1 {
		t.Fatalf("应在一次事务内完成，实际提交 %d 次", tx.Commits())
	}
	if ad, _ := repos.FakeAuthorStats().Delta(author); ad != 1 {
		t.Fatalf("作者文章数应 +1，实际 %d", ad)
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticleCreated {
		t.Fatalf("应发创建事件，实际 %v", names)
	}
}

func TestCreateSubmitGoesToReview(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:         authorViewer(),
		Title:         "待审稿",
		Content:       "正文",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: false,
	})
	if err != nil {
		t.Fatalf("提交审核失败: %v", err)
	}
	if a.Status() != domain.StatusExamining {
		t.Fatalf("应为审核中，实际 %s", a.Status())
	}
	if a.SubmittedAt() == nil {
		t.Fatal("提交审核应记录 submitted_at")
	}

	tasks, err := repos.FakeReviews().ListPendingByArticle(ctx, a.ID())
	if err != nil {
		t.Fatalf("查任务失败: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Source() != review.SourceCreate {
		t.Fatalf("应建 1 条 create 来源的待审任务，实际 %d", len(tasks))
	}
}

func TestCreateSkipExaminingPublishes(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:         authorViewer(),
		Title:         "直发",
		Content:       "正文",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: true,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if a.Status() != domain.StatusPublished {
		t.Fatalf("跳过审核应直接发布，实际 %s", a.Status())
	}
	tasks, _ := repos.FakeReviews().ListPendingByArticle(ctx, a.ID())
	if len(tasks) != 0 {
		t.Fatalf("直接发布不应建审核任务，实际 %d 条", len(tasks))
	}
}

func TestCreateRejectsForeignCategory(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	other := domain.UserID(777)
	cid := domain.CategoryID(5)
	repos.FakeCategories().Seed(apptest.NewCategoryForTest(cid, other, "别人的分类"))

	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Title:        "x",
		Content:      "y",
		CategoryID:   &cid,
		TargetStatus: domain.StatusDraft,
	})
	if err == nil {
		t.Fatal("用别人的分类应报错")
	}
}

func TestCreateRejectsDisabledTag(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()

	tag := apptest.NewTagForTest(1, "废弃", false)
	repos.FakeTags().Seed(tag)

	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Title:        "x",
		Content:      "y",
		TagIDs:       []domain.TagID{1},
		TargetStatus: domain.StatusDraft,
	})
	if err == nil {
		t.Fatal("带停用标签应报错")
	}
	if tx.Rollbacks() == 0 {
		t.Fatal("失败应回滚事务")
	}
	// 回滚后不应留下文章
	if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
		t.Fatalf("回滚后不应有文章，实际 %v", ids)
	}
}

// TestCreateRollsBackOnAuthorStatsFailure 证明"跨模块写失败会连累主流程回滚"。
func TestCreateRollsBackOnAuthorStatsFailure(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	boom := errors.New("user 模块挂了")
	repos.FakeAuthorStats().FailNext("ApplyArticleDelta", boom)

	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Title:        "x",
		Content:      "y",
		TargetStatus: domain.StatusDraft,
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回原始错误，实际 %v", err)
	}
	if tx.Rollbacks() != 1 {
		t.Fatalf("应回滚 1 次，实际 %d", tx.Rollbacks())
	}
	if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
		t.Fatalf("回滚后文章应不存在，实际 %v", ids)
	}
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("回滚后不应留下事件，实际 %v", names)
	}
}

// ── Update ──

func TestUpdateForbiddenForStranger(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusDraft)

	title := "改标题"
	_, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     strangerViewer(),
		ArticleID: 1,
		Title:     &title,
	})
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("他人改文章应报不存在，实际 %v", err)
	}
}

func TestUpdateNoopDoesNotWrite(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusDraft)

	same := "测试文章"
	res, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: 1,
		Title:     &same,
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if res.Changed {
		t.Fatal("值没变不应判定为变更")
	}
	if got := repos.FakeArticles().Calls("Update"); got != 0 {
		t.Fatalf("空操作不应写库，实际调用 Update %d 次", got)
	}
}

// TestUpdatePublishedTriggersReReview 锁住"改已发布文章会自动重新送审"。
func TestUpdatePublishedTriggersReReview(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	content := "改过的正文"
	res, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: 1,
		Content:   &content,
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if !res.Resubmitted {
		t.Fatal("改已发布文章应重新送审")
	}
	if res.Article.Status() != domain.StatusExamining {
		t.Fatalf("状态应为审核中，实际 %s", res.Article.Status())
	}
	tasks, _ := repos.FakeReviews().ListPendingByArticle(ctx, 1)
	if len(tasks) != 1 || tasks[0].Source() != review.SourceEdit {
		t.Fatalf("应建 1 条 edit 来源任务，实际 %d", len(tasks))
	}
}

// TestUpdateVisibilityOnlyDoesNotReReview 是边界：只改可见性不触发重审。
func TestUpdateVisibilityOnlyDoesNotReReview(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	hidden := domain.VisibilityUserHidden
	res, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:      authorViewer(),
		ArticleID:  1,
		Visibility: &hidden,
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if res.Resubmitted {
		t.Fatal("只改可见性不应重新送审")
	}
	if res.Article.Status() != domain.StatusPublished {
		t.Fatalf("状态应保持已发布，实际 %s", res.Article.Status())
	}
}

func TestUpdateBackToDraftCancelsTask(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, 1, author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	draft := domain.StatusDraft
	res, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:        authorViewer(),
		ArticleID:    1,
		TargetStatus: &draft,
	})
	if err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if res.Article.Status() != domain.StatusDraft {
		t.Fatalf("状态应为草稿，实际 %s", res.Article.Status())
	}
	pending, _ := repos.FakeReviews().ListPendingByArticle(ctx, 1)
	if len(pending) != 0 {
		t.Fatalf("退回草稿应撤销待审任务，实际还剩 %d 条", len(pending))
	}
}

// ── Examine ──

func TestExamineRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusExamining)

	_, err := svc.Examine(ctx, app.ExamineCmd{Actor: authorViewer(), ArticleID: 1, Approve: true})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("作者不能审核自己的文章，期望 ErrForbidden，实际 %v", err)
	}
}

func TestExamineRequiresExaminingStatus(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusDraft)

	_, err := svc.Examine(ctx, app.ExamineCmd{Actor: adminViewer(), ArticleID: 1, Approve: true})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("草稿不能审核，期望 ErrInvalidTransition，实际 %v", err)
	}
}

func TestExamineApprove(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, 1, author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	got, err := svc.Examine(ctx, app.ExamineCmd{Actor: adminViewer(), ArticleID: 1, Approve: true})
	if err != nil {
		t.Fatalf("审核通过失败: %v", err)
	}
	if got.Status() != domain.StatusPublished {
		t.Fatalf("应为已发布，实际 %s", got.Status())
	}
	if got.ReviewedBy() == nil || *got.ReviewedBy() != admin {
		t.Fatal("应记录审核人")
	}

	task, err := repos.FakeReviews().Find(ctx, 1)
	if err != nil {
		t.Fatalf("查任务失败: %v", err)
	}
	if task.Status() != review.StatusApproved {
		t.Fatalf("任务应为 approved，实际 %s", task.Status())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticlePublished {
		t.Fatalf("应发发布事件，实际 %v", names)
	}
}

func TestExamineReject(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, 1, author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	got, err := svc.Examine(ctx, app.ExamineCmd{
		Actor: adminViewer(), ArticleID: 1, Approve: false, Reason: "内容不完整",
	})
	if err != nil {
		t.Fatalf("审核拒绝失败: %v", err)
	}
	if got.Status() != domain.StatusRejected {
		t.Fatalf("应为已拒绝，实际 %s", got.Status())
	}
	task, _ := repos.FakeReviews().Find(ctx, 1)
	if task.Reason() != "内容不完整" {
		t.Fatalf("应记录拒绝原因，实际 %q", task.Reason())
	}
}

// ── Delete ──

func TestDeleteCascade(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, 1, author, domain.StatusPublished)

	repos.FakeComments().Seed(a.ID(), 3)
	repos.FakeDiggs().Add(ctx, a.ID(), stranger)
	repos.FakeFavorites().SeedFolder(
		favorite.RestoreFolder(favorite.RestoreFolderParams{ID: 1, UserID: stranger, Title: "默认", IsDefault: true}),
	)
	repos.FakeTags().Seed(apptest.NewTagForTest(7, "Go", true))
	repos.FakeTags().SeedArticleTags(a.ID(), 7)
	repos.FakeTops().Save(ctx, apptest.NewTopEntryForTest(stranger, a.ID()))

	res, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: 1})
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if res.Comments != 3 {
		t.Fatalf("应清理 3 条评论，实际 %d", res.Comments)
	}
	if res.Diggs != 1 {
		t.Fatalf("应清理 1 条点赞，实际 %d", res.Diggs)
	}
	if res.ArticleTags != 1 {
		t.Fatalf("应清理 1 条标签关系，实际 %d", res.ArticleTags)
	}

	// 文章变不可见
	if _, err := repos.FakeArticles().Find(ctx, 1); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("删除后应查不到，实际 %v", err)
	}
	if ad, _ := repos.FakeAuthorStats().Delta(author); ad != -1 {
		t.Fatalf("作者文章数应 -1，实际 %d", ad)
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticleDeleted {
		t.Fatalf("应发删除事件，实际 %v", names)
	}
}

func TestDeleteByStrangerForbidden(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	_, err := svc.Delete(ctx, app.DeleteCmd{Actor: strangerViewer(), ArticleID: 1})
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("他人不能删，期望 ErrArticleNotFound，实际 %v", err)
	}
	if _, err := repos.FakeArticles().Find(ctx, 1); err != nil {
		t.Fatalf("文章应仍然存在: %v", err)
	}
}

func TestDeleteByAdminAllowed(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	if _, err := svc.Delete(ctx, app.DeleteCmd{Actor: adminViewer(), ArticleID: 1}); err != nil {
		t.Fatalf("管理员应能删他人文章: %v", err)
	}
}

// TestDeleteRollsBackOnCommentFailure 是很实际的一条：
// 评论删除失败时，点赞/收藏/统计都不该被改动。
func TestDeleteRollsBackOnCommentFailure(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, 1, author, domain.StatusPublished)
	repos.FakeComments().Seed(a.ID(), 2)
	repos.FakeDiggs().Add(ctx, a.ID(), stranger)
	boom := errors.New("comment 模块挂了")
	repos.FakeComments().FailNext("DeleteByArticle", boom)

	_, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: 1})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回原始错误，实际 %v", err)
	}
	if tx.Rollbacks() != 1 {
		t.Fatalf("应回滚 1 次，实际 %d", tx.Rollbacks())
	}
	// 文章还在
	if _, err := repos.FakeArticles().Find(ctx, 1); err != nil {
		t.Fatalf("回滚后文章应仍存在: %v", err)
	}
	// 点赞没被删
	if n := repos.FakeDiggs().Count(1); n != 1 {
		t.Fatalf("回滚后点赞应仍是 1，实际 %d", n)
	}
	// 作者统计没变
	if ad, _ := repos.FakeAuthorStats().Delta(author); ad != 0 {
		t.Fatalf("回滚后作者统计不应变化，实际 %d", ad)
	}
}

// ── Detail / List ──

func TestDetailHidesDraftFromAnonymous(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusDraft)

	_, err := svc.Detail(ctx, 1, domain.Anonymous())
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("匿名看草稿应报不存在，实际 %v", err)
	}
	if _, err := svc.Detail(ctx, 1, authorViewer()); err != nil {
		t.Fatalf("作者应能看自己的草稿: %v", err)
	}
}

func TestDetailMergesCountersAndFlags(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)
	repos.FakeCounters().Add(ctx, 1, domain.Counters{View: 5, Digg: 2})
	repos.FakeDiggs().Add(ctx, 1, stranger)

	view, err := svc.Detail(ctx, 1, strangerViewer())
	if err != nil {
		t.Fatalf("查详情失败: %v", err)
	}
	if view.Article.Counters().View != 5 {
		t.Fatalf("浏览数应合并缓存增量 5，实际 %d", view.Article.Counters().View)
	}
	if !view.Digg {
		t.Fatal("已点赞应标记")
	}
	if view.Author.Nickname != "作者" {
		t.Fatalf("应带作者昵称，实际 %q", view.Author.Nickname)
	}
}

func TestListAnonymousForcesOnlyPublic(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)
	seedArticle(t, repos, 2, author, domain.StatusDraft)

	views, total, err := svc.List(ctx, domain.Filter{}, domain.Page{Limit: 10}, domain.Anonymous())
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 || len(views) != 1 || views[0].Article.ID() != 1 {
		t.Fatalf("匿名只能看到已发布的 1 篇，实际 total=%d", total)
	}
}

// ── 交互 ──

func TestFavoriteAndUnfavorite(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	if err := svc.Favorite(ctx, app.FavoriteCmd{Actor: strangerViewer(), ArticleID: 1}); err != nil {
		t.Fatalf("收藏失败: %v", err)
	}
	if got := repos.FakeCounters().Get(1).Favor; got != 1 {
		t.Fatalf("收藏增量应为 1，实际 %d", got)
	}
	// 重复收藏应报错
	if err := svc.Favorite(ctx, app.FavoriteCmd{Actor: strangerViewer(), ArticleID: 1}); !errors.Is(err, favorite.ErrItemExists) {
		t.Fatalf("重复收藏应报 ErrItemExists，实际 %v", err)
	}

	if err := svc.Unfavorite(ctx, app.FavoriteCmd{Actor: strangerViewer(), ArticleID: 1}); err != nil {
		t.Fatalf("取消收藏失败: %v", err)
	}
	if got := repos.FakeCounters().Get(1).Favor; got != 0 {
		t.Fatalf("取消后收藏增量应回到 0，实际 %d", got)
	}
}

func TestFavoriteOwnArticleRejected(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	err := svc.Favorite(ctx, app.FavoriteCmd{Actor: authorViewer(), ArticleID: 1})
	if !errors.Is(err, favorite.ErrSelfFavorite) {
		t.Fatalf("不能收藏自己的文章，期望 ErrSelfFavorite，实际 %v", err)
	}
}

func TestDiggToggle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	liked, err := svc.Digg(ctx, 1, strangerViewer())
	if err != nil {
		t.Fatalf("点赞失败: %v", err)
	}
	if !liked || repos.FakeCounters().Get(1).Digg != 1 {
		t.Fatalf("首次点赞应 liked=true 且增量 1，实际 liked=%v delta=%d", liked, repos.FakeCounters().Get(1).Digg)
	}

	liked, err = svc.Digg(ctx, 1, strangerViewer())
	if err != nil {
		t.Fatalf("取消点赞失败: %v", err)
	}
	if liked || repos.FakeCounters().Get(1).Digg != 0 {
		t.Fatalf("再次点击应取消，实际 liked=%v delta=%d", liked, repos.FakeCounters().Get(1).Digg)
	}
}

func TestRecordViewWritesHistoryOnlyForLoggedIn(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedArticle(t, repos, 1, author, domain.StatusPublished)

	if err := svc.RecordView(ctx, 1, domain.Anonymous()); err != nil {
		t.Fatalf("匿名浏览失败: %v", err)
	}
	if got := repos.FakeCounters().Get(1).View; got != 1 {
		t.Fatalf("浏览增量应为 1，实际 %d", got)
	}
	records, total, err := svc.ListViewHistory(ctx, strangerViewer(), domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查历史失败: %v", err)
	}
	if total != 0 || len(records) != 0 {
		t.Fatalf("匿名浏览不应写历史，实际 %d 条", total)
	}

	if err := svc.RecordView(ctx, 1, strangerViewer()); err != nil {
		t.Fatalf("登录用户浏览失败: %v", err)
	}
	records, total, err = svc.ListViewHistory(ctx, strangerViewer(), domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查历史失败: %v", err)
	}
	if total != 1 || len(records) != 1 || records[0].ArticleID() != 1 {
		t.Fatalf("登录用户应写入历史，实际 %d 条", total)
	}
}
