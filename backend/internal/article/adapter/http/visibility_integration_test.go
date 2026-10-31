//go:build integration

package http_test

import (
	"net/http"
	"testing"
	"time"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	"myblogx/internal/testkit/schema"
)

// 本文件验证「显示非管理员发布文章」开关在**真 MySQL** 上的读面效果
// （ADR-0017 决策 4）。DSL 那一半由 search 模块的 golden 守；
// 这里守三个 MySQL 读面：详情、收藏内容、浏览历史。
//
// 为什么必须真库：收藏内容与浏览历史的过滤是 SQL（join user_models 判角色），
// 用内存 fake 只能验证"开关传下去了"，验证不了 join 写对没有。

// TestVisibilitySwitchFiltersMySQLReadSurfaces 走真 HTTP + 真 MySQL，跑开关的开/关两态。
func TestVisibilitySwitchFiltersMySQLReadSurfaces(t *testing.T) {
	db := migrate(t)

	// 普通作者（100）、管理员作者（900）、读者（200）。
	seedGoldenUser(t, db, 100, "vis_author", "普通作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 900, "vis_admin", "管理员作者", "", "", schema.RoleAdmin)
	seedGoldenUser(t, db, 200, "vis_reader", "读者", "", "", schema.RoleUser)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, spec := range []struct {
		id     uint64
		author uint64
		title  string
	}{
		{id: 1001, author: 100, title: "普通作者的文章"},
		{id: 1002, author: 900, title: "管理员作者的文章"},
	} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             spec.id,
			Title:          spec.title,
			AuthorID:       spec.author,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      base,
			UpdatedAt:      base,
		})
	}

	// 读者 200 的默认收藏夹里收了这两篇，浏览历史也各有记录。
	schema.InsertFavorite(t, db, schema.FavoriteSpec{
		ID: 1, UserID: 200, Title: "默认收藏夹", IsDefault: true, ArticleCount: 2, At: base,
	})
	for k, item := range []struct {
		articleID ctype.ID
		authorID  ctype.ID
		title     string
	}{
		{articleID: ctype.NewID(1001), authorID: ctype.NewID(100), title: "普通作者的文章"},
		{articleID: ctype.NewID(1002), authorID: ctype.NewID(900), title: "管理员作者的文章"},
	} {
		schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
			ID:              uint64(5000 + k),
			ArticleID:       item.articleID.Uint64(),
			UserID:          200,
			FavorID:         1,
			ArticleTitle:    item.title,
			ArticleStatus:   schema.ArticleStatusPublished,
			ArticleAuthorID: item.authorID.Uint64(),
			At:              base,
		})
		schema.InsertViewHistory(t, db, uint64(6000+k), 200, item.articleID.Uint64(), base)
	}

	readerOn := newRouterWithSite(t, db, domain.NormalUser(ctype.NewID(200)), 100, nil)
	readerOff := newRouterWithSite(t, db, domain.NormalUser(ctype.NewID(200)), 100,
		siteConfigWithShowNonAdmin(t, false))
	anonOff := newRouterWithSite(t, db, domain.Anonymous(), 100,
		siteConfigWithShowNonAdmin(t, false))

	// ── 开关打开：行为与改造前一致（三篇/两条都在）──
	respCode(t, do(t, readerOn, http.MethodGet, "/api/articles/1001", nil), "开关打开时读普通作者的文章")
	if got := len(listOf(t, do(t, readerOn, http.MethodGet, "/api/articles/favorite/contents?favorite_id=1", nil))); got != 2 {
		t.Fatalf("开关打开时收藏内容应有 2 条，实际 %d", got)
	}
	if got := len(listOf(t, do(t, readerOn, http.MethodGet, "/api/articles/history", nil))); got != 2 {
		t.Fatalf("开关打开时浏览历史应有 2 条，实际 %d", got)
	}

	// ── 开关关闭：非管理员作者的文章从三个读面一致消失 ──
	gone := do(t, anonOff, http.MethodGet, "/api/articles/1001", nil)
	if code, _ := gone["code"].(float64); code == 0 {
		t.Fatalf("开关关闭时匿名不应读到普通作者的文章（应 404 语义）: %v", gone)
	}
	respCode(t, do(t, anonOff, http.MethodGet, "/api/articles/1002", nil), "开关关闭时仍能读管理员作者的文章")

	favorites := listOf(t, do(t, readerOff, http.MethodGet, "/api/articles/favorite/contents?favorite_id=1", nil))
	if len(favorites) != 1 || !containsArticleID(favorites, "1002") {
		t.Fatalf("开关关闭时收藏内容应只剩管理员作者那篇，实际 %v", favorites)
	}
	history := listOf(t, do(t, readerOff, http.MethodGet, "/api/articles/history", nil))
	if len(history) != 1 || !containsArticleID(history, "1002") {
		t.Fatalf("开关关闭时浏览历史应只剩管理员作者那篇，实际 %v", history)
	}

	// ── 作者本人不受影响：studio 编辑器靠同一个详情接口取正文 ──
	ownArticle := newRouterWithSite(t, db, domain.NormalUser(ctype.NewID(100)), 100,
		siteConfigWithShowNonAdmin(t, false))
	respCode(t, do(t, ownArticle, http.MethodGet, "/api/articles/1001", nil), "开关关闭时作者本人读自己的文章")
}

// siteConfigWithShowNonAdmin 造一份"只有默认值、没有库"的运行时配置。
//
// RuntimeConfigService 的缓存为空时 GetRuntimeArticle 回退到 defaults，
// 所以这里不需要 MySQL：正是 App 里"配置没加载完"的同一形态。
func siteConfigWithShowNonAdmin(t *testing.T, show bool) *config.RuntimeConfigService {
	t.Helper()
	return config.NewRuntimeConfigService(
		conf.RuntimeDefaults{Site: conf.Site{Article: siteconf.Article{ShowNonAdminArticles: show}}},
		nil, nil,
	)
}
