package apptest_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
)

// TestTxRollbackRestoresRepos 是本包存在的主要理由：
// 事务失败后，所有仓储必须回到调用前的状态。
// gomock 只能断言"调了 Update"，做不到这一点。
func TestTxRollbackRestoresRepos(t *testing.T) {
	t.Parallel()

	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	ctx := context.Background()

	existing := apptest.NewArticle().ID(1).Author(100).Build()
	repos.FakeArticles().Seed(existing)

	boom := errors.New("下游失败")

	err := tx.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
		// 改一篇已有的
		a, err := r.Articles().Find(ctx, 1)
		if err != nil {
			return err
		}
		a.SoftDelete(apptest.DefaultTime)
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		// 再新建一篇
		if err := r.Articles().Create(ctx, apptest.NewArticle().ID(2).Build()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回原始错误，实际 %v", err)
	}

	if tx.Rollbacks() != 1 {
		t.Fatalf("应记录 1 次回滚，实际 %d", tx.Rollbacks())
	}

	// 删除被撤销：文章 1 仍可读
	if _, err := repos.FakeArticles().Find(ctx, 1); err != nil {
		t.Fatalf("回滚后文章 1 应仍可读: %v", err)
	}
	// 新建被撤销：文章 2 不存在
	if _, err := repos.FakeArticles().Find(ctx, 2); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("回滚后文章 2 不应存在，实际 err=%v", err)
	}
}

func TestTxCommitKeepsChanges(t *testing.T) {
	t.Parallel()

	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	ctx := context.Background()

	err := tx.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
		return r.Articles().Create(ctx, apptest.NewArticle().ID(7).Build())
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if tx.Commits() != 1 || tx.Rollbacks() != 0 {
		t.Fatalf("应提交 1 次、回滚 0 次，实际 提交=%d 回滚=%d", tx.Commits(), tx.Rollbacks())
	}
	if _, err := repos.FakeArticles().Find(ctx, 7); err != nil {
		t.Fatalf("提交后应能读到文章 7: %v", err)
	}
}

func TestTxFailNextSkipsCallback(t *testing.T) {
	t.Parallel()

	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	boom := errors.New("事务没开起来")
	tx.FailNext(boom)

	called := false
	err := tx.WithinTx(context.Background(), func(context.Context, app.Repos) error {
		called = true
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回注入的错误，实际 %v", err)
	}
	if called {
		t.Fatal("注入失败时不应执行回调")
	}
	if tx.Calls() != 1 {
		t.Fatalf("应记录 1 次事务调用，实际 %d", tx.Calls())
	}
}

// TestArticleRepoConflict 测唯一键冲突：Create 同 ID 第二次必须失败。
func TestArticleRepoConflict(t *testing.T) {
	t.Parallel()

	repo := apptest.NewArticleRepo()
	ctx := context.Background()
	a := apptest.NewArticle().ID(1).Build()

	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("首次创建应成功: %v", err)
	}
	err := repo.Create(ctx, a)
	if !errors.Is(err, apptest.ErrConflict) {
		t.Fatalf("重复创建应返回 ErrConflict，实际 %v", err)
	}
}

// TestSnapshotIsolation 保证仓储与调用方不共享指针：
// 测试改了 Find 的返回值，仓储内部不应受影响。
func TestSnapshotIsolation(t *testing.T) {
	t.Parallel()

	repo := apptest.NewArticleRepo().Seed(apptest.NewArticle().ID(1).Title("原始").Build())
	ctx := context.Background()

	got, err := repo.Find(ctx, 1)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	title := "被测试改掉"
	if _, err := got.ApplyEdit(domain.EditPatch{Title: &title}, apptest.DefaultTime); err != nil {
		t.Fatalf("改标题失败: %v", err)
	}

	again, err := repo.Find(ctx, 1)
	if err != nil {
		t.Fatalf("再读失败: %v", err)
	}
	if again.Title() != "原始" {
		t.Fatalf("仓储被外部修改污染，标题变成 %q", again.Title())
	}
}

// TestListFilterOnlyPublic 锁住"前台列表不能漏草稿"这条。
func TestListFilterOnlyPublic(t *testing.T) {
	t.Parallel()

	repo := apptest.NewArticleRepo().Seed(
		apptest.NewArticle().ID(1).Status(domain.StatusPublished).Build(),
		apptest.NewArticle().ID(2).Status(domain.StatusDraft).Build(),
		apptest.NewArticle().ID(3).Status(domain.StatusPublished).Visibility(domain.VisibilityUserHidden).Build(),
	)

	ctx := context.Background()
	got, total, err := repo.List(ctx, domain.Filter{OnlyPublic: true}, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID() != 1 {
		t.Fatalf("只应返回已发布且可见的文章 1，实际 total=%d ids=%v", total, idsOf(got))
	}
}

func TestListFilterByAuthorAndKeyword(t *testing.T) {
	t.Parallel()

	repo := apptest.NewArticleRepo().Seed(
		apptest.NewArticle().ID(1).Author(100).Title("Go 并发").Build(),
		apptest.NewArticle().ID(2).Author(100).Title("Vue 入门").Build(),
		apptest.NewArticle().ID(3).Author(200).Title("Go 泛型").Build(),
	)

	ctx := context.Background()
	author := domain.UserID(100)

	got, total, err := repo.List(ctx, domain.Filter{AuthorID: &author}, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("作者 100 应有 2 篇，实际 %d（%v）", total, idsOf(got))
	}

	got, total, err = repo.List(ctx, domain.Filter{Keyword: "go"}, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("关键字 go 应命中 2 篇，实际 %d（%v）", total, idsOf(got))
	}
}

func TestFaultInjection(t *testing.T) {
	t.Parallel()

	repo := apptest.NewArticleRepo()
	ctx := context.Background()
	boom := errors.New("DB 挂了")
	repo.FailNext("Create", boom)

	if err := repo.Create(ctx, apptest.NewArticle().ID(1).Build()); !errors.Is(err, boom) {
		t.Fatalf("应命中注入故障，实际 %v", err)
	}
	// 一次性：第二次应成功
	if err := repo.Create(ctx, apptest.NewArticle().ID(1).Build()); err != nil {
		t.Fatalf("故障应只触发一次，第二次实际 %v", err)
	}
}

func TestDefaultFolderAutoCreated(t *testing.T) {
	t.Parallel()

	repo := apptest.NewFavoriteRepo()
	ctx := context.Background()

	f, err := repo.DefaultFolder(ctx, 100)
	if err != nil {
		t.Fatalf("应自动创建默认收藏夹: %v", err)
	}
	if !f.IsDefault() || f.UserID() != 100 {
		t.Fatalf("默认收藏夹属性不对: %+v", f)
	}
	// 第二次取到同一个
	again, err := repo.DefaultFolder(ctx, 100)
	if err != nil {
		t.Fatalf("第二次取默认夹失败: %v", err)
	}
	if again.ID() != f.ID() {
		t.Fatalf("应复用同一收藏夹，实际 %d vs %d", again.ID(), f.ID())
	}
}

func TestOutboxRecordsEvents(t *testing.T) {
	t.Parallel()

	o := apptest.NewOutbox()
	ctx := context.Background()
	if err := o.Emit(ctx, app.Event{Name: "article.published"}); err != nil {
		t.Fatalf("发事件失败: %v", err)
	}
	if err := o.Emit(ctx, app.Event{Name: "article.deleted"}); err != nil {
		t.Fatalf("发事件失败: %v", err)
	}
	names := o.Names()
	if len(names) != 2 || names[0] != "article.published" || names[1] != "article.deleted" {
		t.Fatalf("事件记录不符: %v", names)
	}
}

func idsOf(articles []*domain.Article) []domain.ArticleID {
	out := make([]domain.ArticleID, 0, len(articles))
	for _, a := range articles {
		out = append(out, a.ID())
	}
	return out
}
