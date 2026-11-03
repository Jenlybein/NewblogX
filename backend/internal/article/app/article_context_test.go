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

// 本文件是「文章上下文」用例的接缝（有状态 fake 仓储）：
// 阅读页左栏要的那棵树、当前节点、按 DFS 顺序的上一篇/下一篇，以及来源回退。
//
// 只断言用例的可观察结果（树、当前/前后节点、归属），不测私有辅助函数。

func articleNodeID(item *outline.ViewNode) uint64 {
	if item == nil {
		return 0
	}
	return item.Node.ArticleID().Uint64()
}

// seedOutlineNode 往任意一棵树（合集或项目）里播种一行。
func seedOutlineNode(
	t *testing.T, repos *apptest.Repos,
	ownerKind outline.OwnerKind, ownerID uint64,
	nodeID, parentID uint64, kind outline.NodeKind, title string, articleID uint64, order int,
) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(nodeID),
		OwnerKind: ownerKind,
		OwnerID:   ctype.NewID(ownerID),
		ParentID:  ctype.NewID(parentID),
		Kind:      kind,
		Title:     title,
		ArticleID: ctype.NewID(articleID),
		SortOrder: order,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造树节点失败: %v", err)
	}
	repos.FakeOutlines().Seed(n)
	return n
}

// seedDisplayedProject 预置一个展示中的项目（项目树与合集树用同一张表、同一个 owner 维度）。
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

// TestArticleContextFromSpecifiedTreeReturnsNeighboursByDFS 有来源时只给那一棵树，
// 当前节点是这篇文章的节点，上一篇/下一篇按 DFS 顺序取相邻文章（跨文件夹也相邻）。
func TestArticleContextFromSpecifiedTreeReturnsNeighboursByDFS(t *testing.T) {
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

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(12), "collection:900", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if !got.HasTree {
		t.Fatal("有来源时左栏应有树")
	}
	if got.OwnerKind != outline.OwnerCollection || got.OwnerID != ctype.NewID(outlineOwnerID) {
		t.Fatalf("归属应与来源一致，实际 %v/%s", got.OwnerKind, got.OwnerID)
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

// TestArticleContextWithoutSourceFallsBackToFirstOwningTree `from` 缺失时回退到
// 第一棵归属树（合集按 owner_kind 排在项目之前）；当前节点与前后关系仍然成立。
func TestArticleContextWithoutSourceFallsBackToFirstOwingTree(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedDisplayedProject(t, repos, 700, true)

	// 合集 900：11 → 12；项目 700：11。
	seedOutlineNode(t, repos, outline.OwnerCollection, outlineOwnerID, 1, 0, outline.NodeKindArticle, "", 11, 1)
	seedOutlineNode(t, repos, outline.OwnerCollection, outlineOwnerID, 2, 0, outline.NodeKindArticle, "", 12, 2)
	seedOutlineNode(t, repos, outline.OwnerProject, 700, 3, 0, outline.NodeKindArticle, "", 11, 1)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(11), "", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if !got.HasTree {
		t.Fatal("有归属树时应返回树")
	}
	if got.OwnerKind != outline.OwnerCollection || got.OwnerID != ctype.NewID(outlineOwnerID) {
		t.Fatalf("应回退到第一棵归属树（合集 900），实际 %v/%s", got.OwnerKind, got.OwnerID)
	}
	if articleNodeID(got.Current) != 11 || articleNodeID(got.Next) != 12 {
		t.Fatalf("当前/下一篇不对: current=%d next=%d", articleNodeID(got.Current), articleNodeID(got.Next))
	}
}

// TestArticleContextSpecifiedTreeIsHiddenIsEmpty 来源指向未展示的合集时按不存在处理：
// 不泄露隐藏专题的树，也不"串"到这篇文档的另一棵树。
func TestArticleContextSpecifiedTreeIsHiddenIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedDisplayedProject(t, repos, 700, true)
	seedHiddenCollection(t, repos, outlineOwnerID)

	// 两份归属：隐藏的合集 900 与展示中的项目 700。
	seedOutlineNode(t, repos, outline.OwnerCollection, outlineOwnerID, 1, 0, outline.NodeKindArticle, "", 11, 1)
	seedOutlineNode(t, repos, outline.OwnerProject, 700, 2, 0, outline.NodeKindArticle, "", 11, 1)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(11), "collection:900", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if got.HasTree {
		t.Fatalf("未展示来源的树不应返回，实际 %+v", got)
	}
}

// TestArticleContextWithoutAnyOwningTreeIsEmpty 文章不属于任何树时返回空结果：
// 页面据此不渲染左栏（Content 不受影响）。
func TestArticleContextWithoutAnyOwningTreeIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(13), "", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if got.HasTree || got.Current != nil || len(got.Tree) != 0 {
		t.Fatalf("无归属树时应返回空结果，实际 %+v", got)
	}
}

// TestArticleContextSpecifiedTreeWithoutArticleIsEmpty 来源指向的树里没有这篇文章时
// 不跨树回退：读者从 A 专题点进来，不该被塞进 B 专题的树。
func TestArticleContextSpecifiedTreeWithoutArticleIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedDisplayedProject(t, repos, 700, true)

	// 文章 13 只在项目 700 里，合集 900 里没有它。
	seedOutlineNode(t, repos, outline.OwnerCollection, outlineOwnerID, 1, 0, outline.NodeKindArticle, "", 11, 1)
	seedOutlineNode(t, repos, outline.OwnerProject, 700, 2, 0, outline.NodeKindArticle, "", 13, 1)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(13), "collection:900", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if got.HasTree {
		t.Fatalf("来源树里没有这篇文章时不应回退到别的树，实际 %+v", got)
	}
}

// TestArticleContextNeighboursSkipFilteredArticleNodes 上一篇/下一篇是**读者树里**
// 相邻的公开文章节点：中间夹着的失效文章节点被过滤掉，不参与相邻关系。
func TestArticleContextNeighboursSkipFilteredArticleNodes(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	// 12 号文章退回草稿：作者树过滤掉之后，11 的下一篇应是 13。
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(admin)).
			Title("草稿篇").Status(domain.StatusDraft).Visibility(domain.VisibilityVisible).Build(),
	)
	repos.FakeOutlines().Seed(
		outlineArticle(t, 1, 0, 11, 1),
		outlineArticle(t, 2, 0, 12, 2),
		outlineArticle(t, 3, 0, 13, 3),
	)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(11), "collection:900", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if got, want := readerDetailLabels(got.Tree), []string{"第一篇", "第三篇"}; !sameStrings(got, want) {
		t.Fatalf("失效节点应从读者树里过滤:\n got %v\nwant %v", got, want)
	}
	if articleNodeID(got.Next) != 13 {
		t.Fatalf("下一篇应跳过失效的文章 12，实际 %d", articleNodeID(got.Next))
	}
}

// TestArticleContextFallsBackPastHiddenTree 无来源回退时跳过对读者隐藏的树：
// 隐藏合集里也挂着这篇文章，但读者只能看到展示中的项目树。
func TestArticleContextFallsBackPastHiddenTree(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	seedDisplayedProject(t, repos, 700, true)
	seedHiddenCollection(t, repos, outlineOwnerID)

	seedOutlineNode(t, repos, outline.OwnerCollection, outlineOwnerID, 1, 0, outline.NodeKindArticle, "", 11, 1)
	seedOutlineNode(t, repos, outline.OwnerProject, 700, 2, 0, outline.NodeKindArticle, "", 11, 1)

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(11), "", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if !got.HasTree {
		t.Fatal("应回退到展示中的项目树")
	}
	if got.OwnerKind != outline.OwnerProject || got.OwnerID != ctype.NewID(700) {
		t.Fatalf("隐藏合集应被跳过，实际 %v/%s", got.OwnerKind, got.OwnerID)
	}
}

// TestArticleContextInvalidSourceFallsBack 来源格式不合法按"没有来源"处理，
// 交给同一段回退逻辑（不在 handler 里再写一份格式判定）。
func TestArticleContextInvalidSourceFallsBack(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	repos.FakeOutlines().Seed(outlineArticle(t, 1, 0, 11, 1))

	got, err := svc.ArticleOutlineContext(ctx, ctype.NewID(11), "not-a-source", false)
	if err != nil {
		t.Fatalf("读文章上下文失败: %v", err)
	}
	if !got.HasTree || got.OwnerKind != outline.OwnerCollection || got.OwnerID != ctype.NewID(outlineOwnerID) {
		t.Fatalf("非法来源应回退到第一棵归属树，实际 %+v", got)
	}
}
