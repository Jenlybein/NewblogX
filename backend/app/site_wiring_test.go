package app

import (
	"context"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	articleapp "myblogx/internal/article/app"
	articleapptest "myblogx/internal/article/apptest"
	articledomain "myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
)

// 本文件锁死右栏推荐合集卡的接线：篇数与封面回退来自 article 模块的卡片读模型
// （目录树），站点不再自己算；「显示非管理员发布文章」开关按现读闭包传下去。

// seedSidebarArticleNode 往合集树里挂一个文章节点。
func seedSidebarArticleNode(
	t *testing.T, repos *articleapptest.Repos, collectionID ctype.ID, nodeID, articleID uint64, order int,
) {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(nodeID),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   outline.ID(collectionID),
		Kind:      outline.NodeKindArticle,
		ArticleID: ctype.NewID(articleID),
		SortOrder: order,
		Now:       articleapptest.DefaultTime,
	})
	if err != nil {
		t.Fatalf("构造目录树节点失败: %v", err)
	}
	repos.FakeOutlines().Seed(n)
}

// TestSidebarCollectionReaderUsesArticleCardModel 右栏合集卡走 article 读模型：
// 树里有公开文章、旧中间表为空时也返回正确篇数与封面回退，开关关闭时按作者角色过滤。
func TestSidebarCollectionReaderUsesArticleCardModel(t *testing.T) {
	t.Parallel()

	repos := articleapptest.NewRepos()
	tx := articleapptest.NewTx(repos)
	svc := articleapp.New(tx, articleapptest.NewClock(articleapptest.DefaultTime), articleapptest.NewIDGen(1000), nil)
	ctx := context.Background()

	adminViewer := articledomain.AdminUser(articledomain.UserID(ctype.NewID(900)))
	collectionID, err := svc.CreateCollection(ctx, articleapp.CreateCollectionCmd{
		Actor: adminViewer, Title: "从零搭博客", Displayed: true,
	})
	if err != nil {
		t.Fatalf("建合集失败: %v", err)
	}

	// 首节点普通作者（开关关闭时不可见）、第二节点管理员作者。
	repos.FakeArticles().Seed(
		articleapptest.NewArticle().ID(ctype.NewID(11)).Author(ctype.ID("100")).
			Status(articledomain.StatusPublished).Visibility(articledomain.VisibilityVisible).
			Cover("https://cdn.example/normal.png").Build(),
		articleapptest.NewArticle().ID(ctype.NewID(12)).Author(ctype.ID("900")).
			Status(articledomain.StatusPublished).Visibility(articledomain.VisibilityVisible).
			Cover("https://cdn.example/admin.png").Build(),
	)
	repos.FakeUsers().Seed(articleapp.AuthorView{ID: ctype.ID("900"), Admin: true})
	seedSidebarArticleNode(t, repos, ctype.ID(collectionID), 1, 11, 1)
	seedSidebarArticleNode(t, repos, ctype.ID(collectionID), 2, 12, 2)

	// 开关打开（site 未装配按默认 true）：两篇都算，封面取首节点。
	open := &sidebarCollectionReader{svc: svc}
	got, err := open.RecommendedCollections(ctx, 5)
	if err != nil {
		t.Fatalf("读右栏合集失败: %v", err)
	}
	if len(got) != 1 || got[0].ArticleCount != 2 ||
		got[0].Cover != "https://cdn.example/normal.png" {
		t.Fatalf("开关打开时右栏合集卡不对: %+v", got)
	}

	// 开关关闭：只剩管理员作者那篇，封面跳过不可见节点。
	closed := &sidebarCollectionReader{svc: svc, effective: config.NewEffectiveConfig(
		config.NewRuntimeConfigService(
			conf.RuntimeDefaults{Site: conf.Site{Article: siteconf.Article{ShowNonAdminArticles: false}}},
			nil, nil,
		),
	)}
	got, err = closed.RecommendedCollections(ctx, 5)
	if err != nil {
		t.Fatalf("读右栏合集失败: %v", err)
	}
	if len(got) != 1 || got[0].ArticleCount != 1 ||
		got[0].Cover != "https://cdn.example/admin.png" {
		t.Fatalf("开关关闭时右栏合集卡不对: %+v", got)
	}
}
