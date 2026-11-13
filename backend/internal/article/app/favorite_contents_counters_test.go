package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/kernel/ctype"
)

// 本文件守 ADR-0016 决策 3：收藏内容列表与详情 / 列表共用一个读口径
// ——「库内列 + Redis 增量」，同一篇文章在三个读面里必须是同一个数字。
//
// 曾经这里直接读 join 出来的列、不并增量，于是"详情页 6、收藏列表 5"。

// TestListFavoriteItemsMergesCounterDeltas 收藏内容列表把增量叠进四个计数。
//
// 断言方式是拿详情读面做对照：两者必须一致。任何"只改一处"的实现都会红。
func TestListFavoriteItemsMergesCounterDeltas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	folderID := ctype.NewID(10)
	articleID := ctype.NewID(1)

	svc, repos, _ := newService(t)

	// 库内快照（生产里两个读面都从 article_models 取这四个数）。
	columns := domain.Counters{View: 6, Digg: 5, Comment: 4, Favor: 3}
	delta := domain.Counters{View: 1, Digg: 2, Comment: 1, Favor: 3}
	want := domain.Counters{View: 7, Digg: 7, Comment: 5, Favor: 6}

	article := apptest.NewArticle().ID(articleID).Author(author).Status(domain.StatusPublished).Build()
	// 聚合默认计数为 0；直接放库内快照（读侧由账本负责"列 + 增量"，聚合只携带）。
	article.SetCounters(columns)
	repos.FakeArticles().Seed(article)

	repos.FakeFavorites().SeedFolder(favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: folderID, UserID: author, Title: "默认收藏夹", IsDefault: true,
	}))
	if err := repos.FakeFavorites().SaveItem(ctx, favorite.RestoreItem(favorite.RestoreItemParams{
		ArticleID:    domain.ArticleID(articleID),
		UserID:       author,
		FolderID:     folderID,
		ArticleTitle: "被收藏的文章",
		AuthorID:     author,
		ViewCount:    columns.View,
		DiggCount:    columns.Digg,
		CommentCount: columns.Comment,
		FavorCount:   columns.Favor,
	})); err != nil {
		t.Fatalf("预置收藏失败: %v", err)
	}
	if err := repos.FakeCounters().Add(ctx, domain.ArticleID(articleID), delta); err != nil {
		t.Fatalf("预置增量失败: %v", err)
	}

	items, total, err := svc.ListFavoriteItems(ctx, authorViewer(), folderID, domain.Page{Limit: 10}, false)
	if err != nil {
		t.Fatalf("列收藏内容失败: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("应恰好 1 条收藏，实际 total=%d len=%d", total, len(items))
	}
	got := items[0]
	if c := want; got.ViewCount() != c.View || got.DiggCount() != c.Digg ||
		got.CommentCount() != c.Comment || got.FavorCount() != c.Favor {
		t.Fatalf("收藏列表计数应为「列 + 增量」%+v，实际 view=%d digg=%d comment=%d favor=%d",
			c, got.ViewCount(), got.DiggCount(), got.CommentCount(), got.FavorCount())
	}

	// 同一个数字：详情读面必须给出同样四项。
	view, err := svc.Detail(ctx, domain.ArticleID(articleID), authorViewer(), false)
	if err != nil {
		t.Fatalf("查详情失败: %v", err)
	}
	if d := view.Article.Counters(); d != want {
		t.Fatalf("详情读面计数应与收藏列表一致 %+v，实际 %+v", want, d)
	}
}
