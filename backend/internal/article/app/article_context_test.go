package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件是具名树端点在阅读页左栏的接缝（有状态 fake 仓储）：
// 归属由端点身份给出，只认 `?from=` 指向的那棵树；当前节点与按 DFS 顺序的
// 上一篇 / 下一篇都从这一棵树里算。旧共享端点「无来源时反查第一棵归属树」
// 的回退已随 #277 下线，不再有测试用例。
//
// 只断言用例的可观察结果（树、当前/前后节点、归属），不测私有辅助函数。

func articleNodeID(item *outline.ViewNode) uint64 {
	if item == nil {
		return 0
	}
	return item.Node.ArticleID().Uint64()
}

// seedDisplayedProject 预置一个展示中的项目。
func seedDisplayedProject(t *testing.T, repos *apptest.Repos, id uint64, displayed bool) {
	t.Helper()
	p, err := project.New(project.NewParams{
		ID:        ctype.NewID(id),
		Title:     "项目",
		CreatorID: domain.UserID(admin),
		Displayed: displayed,
		SortOrder: 1,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造项目失败: %v", err)
	}
	repos.FakeProjects().SeedProject(p)
}

// seedHiddenCollection 用同一个 ID 覆写一个"未展示"的合集（对读者不可见）。
func seedHiddenCollection(t *testing.T, repos *apptest.Repos, id uint64) {
	t.Helper()
	c, err := collection.New(collection.NewParams{
		ID:        ctype.NewID(id),
		Title:     "未展示专题",
		CreatorID: domain.UserID(admin),
		Displayed: false,
		SortOrder: 1,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造未展示合集失败: %v", err)
	}
	repos.FakeCollections().SeedCollection(c)
}

// TestNamedTreeFromSpecifiedOwnerReturnsNeighboursByDFS 来路指向这一棵树时，
// 当前节点与上一篇/下一篇按 DFS 顺序取相邻文章（跨文件夹也相邻）。
func TestNamedTreeFromSpecifiedOwnerReturnsNeighboursByDFS(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	repos.FakeOutlines().Seed(
		outlineFolder(t, 1, 0, "第一章", 1),
		outlineArticle(t, 2, 1, 11, 1),
		outlineArticle(t, 3, 1, 12, 2),
		outlineArticle(t, 4, 0, 13, 2),
	)

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"collection:900", ctype.NewID(12), false, domain.Anonymous())
	if err != nil {
		t.Fatalf("读具名树失败: %v", err)
	}
	if !got.HasTree {
		t.Fatal("来路正确时左栏应有树")
	}
	if got.OwnerKind != outline.OwnerCollection || got.OwnerID != ctype.NewID(outlineOwnerID) {
		t.Fatalf("归属应由端点身份给出，实际 %v/%s", got.OwnerKind, got.OwnerID)
	}
	if got, want := readerDetailLabels(got.Tree), []string{"第一章", "第一篇", "第二篇", "第三篇"}; !sameStrings(got, want) {
		t.Fatalf("树应按 DFS 顺序:\n got %v\nwant %v", got, want)
	}
	if articleNodeID(got.Current) != 12 {
		t.Fatalf("当前节点应是文章 12，实际 %d", articleNodeID(got.Current))
	}
	if articleNodeID(got.Previous) != 11 {
		t.Fatalf("上一篇应是文章 11（同文件夹），实际 %d", articleNodeID(got.Previous))
	}
	if articleNodeID(got.Next) != 13 {
		t.Fatalf("下一篇应是文章 13（跨文件夹后第一篇），实际 %d", articleNodeID(got.Next))
	}
}

// TestNamedTreeRejectsForeignSource 来路指向别的归属时按没有可展示的树处理，
// 不串到读者点进来的那棵之外的专题。
func TestNamedTreeRejectsForeignSource(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedDisplayedProject(t, repos, 700, true)

	repos.FakeOutlines().Seed(outlineArticle(t, 1, 0, 11, 1))

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"project:700", ctype.NewID(11), false, domain.Anonymous())
	if err != nil {
		t.Fatalf("读具名树失败: %v", err)
	}
	if got.HasTree || len(got.Tree) != 0 {
		t.Fatalf("来路指向别的树时不应返回树，实际 %+v", got)
	}
}

// TestNamedTreeSpecifiedOwnerIsHiddenIsEmpty 归属未展示时按不存在处理：
// 不泄露隐藏专题的树。
func TestNamedTreeSpecifiedOwnerIsHiddenIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedHiddenCollection(t, repos, outlineOwnerID)

	repos.FakeOutlines().Seed(outlineArticle(t, 1, 0, 11, 1))

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"collection:900", ctype.NewID(11), false, domain.Anonymous())
	if err != nil {
		t.Fatalf("读具名树失败: %v", err)
	}
	if got.HasTree {
		t.Fatalf("未展示归属的树不应返回，实际 %+v", got)
	}
}

// TestNamedTreeWithoutTheArticleIsEmpty 树里没有这篇文章时不回答相邻文章。
func TestNamedTreeWithoutTheArticleIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	repos.FakeOutlines().Seed(outlineArticle(t, 1, 0, 11, 1))

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"collection:900", ctype.NewID(13), false, domain.Anonymous())
	if err != nil {
		t.Fatalf("读具名树失败: %v", err)
	}
	if got.Current != nil || got.Previous != nil || got.Next != nil {
		t.Fatalf("文章不在树里时相邻文章应全为空，实际 %+v", got)
	}
}

// TestNamedTreeNeighboursSkipFilteredArticleNodes 上一篇/下一篇是**读者树里**
// 相邻的公开文章节点：中间夹着的失效文章节点被过滤掉，不参与相邻关系。
func TestNamedTreeNeighboursSkipFilteredArticleNodes(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(admin)).
			Title("草稿篇").Status(domain.StatusDraft).Visibility(domain.VisibilityVisible).Build(),
	)
	repos.FakeOutlines().Seed(
		outlineArticle(t, 1, 0, 11, 1),
		outlineArticle(t, 2, 0, 12, 2),
		outlineArticle(t, 3, 0, 13, 3),
	)

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"collection:900", ctype.NewID(11), false, domain.Anonymous())
	if err != nil {
		t.Fatalf("读具名树失败: %v", err)
	}
	if got, want := readerDetailLabels(got.Tree), []string{"第一篇", "第三篇"}; !sameStrings(got, want) {
		t.Fatalf("失效节点应从读者树里过滤:\n got %v\nwant %v", got, want)
	}
	if articleNodeID(got.Next) != 13 {
		t.Fatalf("下一篇应跳过失效的文章 12，实际 %d", articleNodeID(got.Next))
	}
}

// TestNamedTreeAdminSeesHiddenOwner 管理员在同一具名端点能读回未展示归属的
// 整棵后台树（与读者详情同一口径的有意例外）。
func TestNamedTreeAdminSeesHiddenOwner(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedHiddenCollection(t, repos, outlineOwnerID)

	repos.FakeOutlines().Seed(outlineArticle(t, 1, 0, 11, 1))

	got, err := svc.NamedOutlineTree(ctx, outline.OwnerCollection, ctype.NewID(outlineOwnerID),
		"", "", false, adminViewer())
	if err != nil {
		t.Fatalf("管理员读具名树失败: %v", err)
	}
	if !got.HasTree || len(got.Tree) != 1 {
		t.Fatalf("管理员应读回整棵后台树，实际 %+v", got)
	}
}
