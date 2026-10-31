package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
)

// 本文件锁定 ADR-0017 决策 4 的第三层语义：「显示非管理员发布文章」开关关闭后，
// 详情 / 收藏内容 / 浏览历史这三个 MySQL 读面与 ES 那三个搜索读面保持一致。
//
// 开关状态由 adapter 从站点运行时配置**现读**后作为普通布尔传进 app，
// 所以这里可以直接把两种形态都跑一遍，不需要起配置服务。

const adminAuthorID = ctype.ID("300")

// TestDetailHidesNonAdminAuthorWhenSwitchOff 是详情读面的可见性矩阵。
//
// 三件事同时被锁住：
//   - 开关打开：作者角色不影响可读性（与改造前一致）；
//   - 开关关闭：非管理员作者的文章按"不存在"处理（404 而不是 403 —— 不给
//     "这篇文章存在但你无权"的可探测信号）；
//   - 开关关闭时，作者本人仍能读到自己的文章（studio 编辑器靠同一个接口取正文），
//     而管理员读者**没有例外**（ADR：不做前台管理员例外）。
func TestDetailHidesNonAdminAuthorWhenSwitchOff(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 普通作者（author=100）与管理员作者（300）各写一篇已发布可见的文章。
	svc, repos, _ := newService(t)
	repos.FakeUsers().Seed(app.AuthorView{ID: adminAuthorID, Nickname: "管理员作者", Admin: true})
	seedArticle(t, repos, ctype.NewID(1), author, domain.StatusPublished)
	seedArticle(t, repos, ctype.NewID(2), adminAuthorID, domain.StatusPublished)

	cases := []struct {
		name string
		id   ctype.ID
		// onlyAdminAuthors 对应「显示非管理员发布文章」= false。
		onlyAdminAuthors bool
		viewer           domain.Viewer
		wantNotFound     bool
	}{
		{name: "开关打开×普通作者×路人", id: ctype.NewID(1), onlyAdminAuthors: false, viewer: strangerViewer()},
		{name: "开关打开×普通作者×匿名", id: ctype.NewID(1), onlyAdminAuthors: false, viewer: domain.Anonymous()},
		{name: "开关关闭×普通作者×路人", id: ctype.NewID(1), onlyAdminAuthors: true, viewer: strangerViewer(), wantNotFound: true},
		{name: "开关关闭×普通作者×匿名", id: ctype.NewID(1), onlyAdminAuthors: true, viewer: domain.Anonymous(), wantNotFound: true},
		{name: "开关关闭×普通作者×作者本人（studio 要能继续编辑）", id: ctype.NewID(1), onlyAdminAuthors: true, viewer: authorViewer()},
		{name: "开关关闭×普通作者×管理员读者没有例外", id: ctype.NewID(1), onlyAdminAuthors: true, viewer: adminViewer(), wantNotFound: true},
		{name: "开关关闭×管理员作者×路人", id: ctype.NewID(2), onlyAdminAuthors: true, viewer: strangerViewer()},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			view, err := svc.Detail(ctx, domain.ArticleID(c.id), c.viewer, c.onlyAdminAuthors)
			if c.wantNotFound {
				if !errors.Is(err, domain.ErrArticleNotFound) {
					t.Fatalf("应返回文章不存在，实际 err=%v view=%+v", err, view)
				}
				return
			}
			if err != nil {
				t.Fatalf("应读到文章，实际 %v", err)
			}
			if view == nil || view.Article.ID() != domain.ArticleID(c.id) {
				t.Fatalf("读到的文章不对: %+v", view)
			}
		})
	}
}

// TestFavoriteItemsThreadReadFilter 证明收藏内容列表把开关交给了查询层。
//
// 过滤必须发生在 SQL 里而不是 app 里：先查一页再在内存里丢掉几行，
// total 会与可见条目对不上（前端分页直接错乱）。因此 app 的职责只是把
// 「只出管理员作者」这个布尔传下去；SQL 的真实过滤由
// adapter/mysql 的集成测试（social_repo_integration_test.go）守。
func TestFavoriteItemsThreadReadFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	folderID := ctype.NewID(10)

	cases := []struct {
		name             string
		onlyAdminAuthors bool
	}{
		{name: "开关打开", onlyAdminAuthors: false},
		{name: "开关关闭", onlyAdminAuthors: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, repos, _ := newService(t)
			repos.FakeFavorites().SeedFolder(favorite.RestoreFolder(favorite.RestoreFolderParams{
				ID: folderID, UserID: author, Title: "默认收藏夹", IsDefault: true,
			}))
			item := favorite.RestoreItem(favorite.RestoreItemParams{
				ArticleID:    domain.ArticleID(ctype.NewID(1)),
				UserID:       author,
				FolderID:     folderID,
				ArticleTitle: "被收藏的文章",
				AuthorID:     author,
			})
			if err := repos.FakeFavorites().SaveItem(ctx, item); err != nil {
				t.Fatalf("预置收藏失败: %v", err)
			}

			if _, _, err := svc.ListFavoriteItems(ctx, authorViewer(), folderID, domain.Page{Limit: 10}, c.onlyAdminAuthors); err != nil {
				t.Fatalf("列收藏内容失败: %v", err)
			}
			if got := repos.FakeFavorites().ListItemsOnlyAdminAuthors(); got != c.onlyAdminAuthors {
				t.Fatalf("查询层收到的过滤应等于开关状态: got=%v want=%v", got, c.onlyAdminAuthors)
			}
		})
	}
}

// TestViewHistoryThreadReadFilter 证明浏览历史把开关交给了查询层（同收藏内容）。
func TestViewHistoryThreadReadFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cases := []struct {
		name             string
		onlyAdminAuthors bool
	}{
		{name: "开关打开", onlyAdminAuthors: false},
		{name: "开关关闭", onlyAdminAuthors: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, repos, _ := newService(t)
			if err := repos.FakeHistory().Upsert(ctx,
				viewhistory.NewRecord(author, domain.ArticleID(ctype.NewID(1)), apptest.DefaultTime)); err != nil {
				t.Fatalf("预置浏览历史失败: %v", err)
			}

			if _, _, err := svc.ListViewHistory(ctx, authorViewer(), domain.Page{Limit: 10}, c.onlyAdminAuthors); err != nil {
				t.Fatalf("列浏览历史失败: %v", err)
			}
			if got := repos.FakeHistory().ListOnlyAdminAuthors(); got != c.onlyAdminAuthors {
				t.Fatalf("查询层收到的过滤应等于开关状态: got=%v want=%v", got, c.onlyAdminAuthors)
			}
		})
	}
}
