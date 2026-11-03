package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件是目录树**应用层**（有状态 fake 仓储）的契约：
// 新建文件夹、批量收录、整树提交幂等、读回嵌套树、失效行保留、权限。
//
// 只断言可观察结果（读回的树、fake 里的节点状态），不测私有辅助函数。

const outlineOwnerID uint64 = 900

var outlineNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func seedCollectionOutlines(t *testing.T) (*app.Service, *apptest.Repos, *apptest.Tx) {
	t.Helper()
	svc, repos, tx := newService(t)

	c, err := collection.New(collection.NewParams{
		ID:        ctype.NewID(outlineOwnerID),
		Title:     "专题",
		CreatorID: domain.UserID(admin),
		Displayed: true,
		SortOrder: 1,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造合集失败: %v", err)
	}
	repos.FakeCollections().SeedCollection(c)
	return svc, repos, tx
}

func outlineFolder(t *testing.T, id, parentID uint64, title string, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindFolder,
		Title:     title,
		SortOrder: order,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造文件夹失败: %v", err)
	}
	return n
}

func outlineArticle(t *testing.T, id, parentID, articleID uint64, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindArticle,
		ArticleID: ctype.NewID(articleID),
		SortOrder: order,
		Now:       outlineNow,
	})
	if err != nil {
		t.Fatalf("构造文章节点失败: %v", err)
	}
	return n
}

func seedOutlineArticle(
	t *testing.T, repos *apptest.Repos, id uint64, title string, status domain.Status,
) *domain.Article {
	t.Helper()
	a := apptest.NewArticle().
		ID(ctype.NewID(id)).
		Author(domain.UserID(admin)).
		Title(title).
		Status(status).
		Build()
	repos.FakeArticles().Seed(a)
	return a
}

// viewLabels 把后台读回的嵌套树拍平成 "缩进 + 标题"，文章节点用标题。
func viewLabels(tree []outline.ViewNode) []string {
	var out []string
	var walk func(items []outline.ViewNode)
	walk = func(items []outline.ViewNode) {
		for _, item := range items {
			label := item.Node.Title()
			if item.Node.IsArticle() {
				label = item.ArticleTitle
			}
			out = append(out, label)
			walk(item.Children)
		}
	}
	walk(tree)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAdminOutlineReturnsNestedTreeWithEmptyFolder 断言后台读回的是嵌套树，
// 空文件夹保留，兄弟顺序稳定。
func TestAdminOutlineReturnsNestedTreeWithEmptyFolder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
	seedOutlineArticle(t, repos, 12, "第二篇", domain.StatusPublished)

	repos.FakeOutlines().Seed(
		outlineFolder(t, 1, 0, "第一章", 1),
		outlineArticle(t, 2, 1, 11, 1),
		outlineFolder(t, 3, 0, "空章节", 2),
		outlineArticle(t, 4, 0, 12, 3),
	)

	detail, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("读回目录树失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"第一章", "第一篇", "空章节", "第二篇"}; !sameStrings(got, want) {
		t.Fatalf("嵌套树不对:\n got %v\nwant %v", got, want)
	}
	if len(detail.Tree[0].Children) != 1 {
		t.Fatalf("第一章应有 1 个文章节点，实际 %d", len(detail.Tree[0].Children))
	}
	if len(detail.Tree[1].Children) != 0 {
		t.Fatalf("空章节应保留且没有子节点")
	}
}

// TestSaveOutlineCreatesFoldersAndIncludesArticles 是后台最小闭环的主线：
// 新建文件夹 + 批量收录 + 读回。
func TestSaveOutlineCreatesFoldersAndIncludesArticles(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
	seedOutlineArticle(t, repos, 12, "第二篇", domain.StatusPublished)

	detail, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
		Actor:     adminViewer(),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{
				Kind:  outline.NodeKindFolder,
				Title: "第一章",
				Children: []app.OutlineInputNode{
					{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
					{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(12)},
				},
			},
			{Kind: outline.NodeKindFolder, Title: "空章节"},
		},
	})
	if err != nil {
		t.Fatalf("保存目录树失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"第一章", "第一篇", "第二篇", "空章节"}; !sameStrings(got, want) {
		t.Fatalf("保存后的树不对:\n got %v\nwant %v", got, want)
	}

	// 每个新节点都有 ID：页面保存后可以立刻拿 ID 继续操作。
	for _, node := range detail.Tree {
		if node.Node.ID().IsZero() {
			t.Fatalf("保存后文件夹应有 ID: %+v", node.Node.Title())
		}
	}

	// 重新打开读回同一棵树。
	again, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("重新读取失败: %v", err)
	}
	if got, want := viewLabels(again.Tree), []string{"第一章", "第一篇", "第二篇", "空章节"}; !sameStrings(got, want) {
		t.Fatalf("重新打开的树与保存结果不一致:\n got %v\nwant %v", got, want)
	}
	if len(repos.FakeOutlines().ActiveNodes()) != 4 {
		t.Fatalf("应有 4 个活跃节点，实际 %d", len(repos.FakeOutlines().ActiveNodes()))
	}
}

// TestSaveOutlineMovesNodesAndReorderSiblings 断言整树提交可以把文章节点
// 从一个文件夹移到另一个文件夹，并调整兄弟顺序（上移 / 下移 / 缩进 / 移出的落库结果）。
func TestSaveOutlineMovesNodesAndReorderSiblings(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
	seedOutlineArticle(t, repos, 12, "第二篇", domain.StatusPublished)

	repos.FakeOutlines().Seed(
		outlineFolder(t, 1, 0, "第一章", 1),
		outlineArticle(t, 2, 1, 11, 1),
		outlineFolder(t, 3, 0, "第二章", 2),
		outlineArticle(t, 4, 0, 12, 3),
	)

	// 把 A11 从第一章移到第二章；把 A12 移到第一章；根层顺序改成 第二章、第一章。
	detail, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
		Actor:     adminViewer(),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{
				ID:    ctype.NewID(3),
				Kind:  outline.NodeKindFolder,
				Title: "第二章",
				Children: []app.OutlineInputNode{
					{ID: ctype.NewID(2), Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
				},
			},
			{
				ID:    ctype.NewID(1),
				Kind:  outline.NodeKindFolder,
				Title: "第一章",
				Children: []app.OutlineInputNode{
					{ID: ctype.NewID(4), Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(12)},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("整树提交失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"第二章", "第一篇", "第一章", "第二篇"}; !sameStrings(got, want) {
		t.Fatalf("移动后的树不对:\n got %v\nwant %v", got, want)
	}
	for _, n := range repos.FakeOutlines().ActiveNodes() {
		switch n.ID() {
		case ctype.NewID(2):
			if n.ParentID() != ctype.NewID(3) || n.SortOrder() != 1 {
				t.Fatalf("A11 应挂到第二章第 1 位，实际 parent=%s order=%d", n.ParentID(), n.SortOrder())
			}
		case ctype.NewID(4):
			if n.ParentID() != ctype.NewID(1) || n.SortOrder() != 1 {
				t.Fatalf("A12 应挂到第一章第 1 位，实际 parent=%s order=%d", n.ParentID(), n.SortOrder())
			}
		}
	}
}

// TestSaveOutlineRenamesExistingFolder 断言带 ID 的文件夹节点改名会落库。
// 页面提交的是整棵树：标题是节点内容的一部分，不能被"复用已有行"吞掉。
func TestSaveOutlineRenamesExistingFolder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	repos.FakeOutlines().Seed(outlineFolder(t, 1, 0, "第一章", 1))

	cmd := app.SaveOutlineCmd{
		Actor: adminViewer(), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{ID: ctype.NewID(1), Kind: outline.NodeKindFolder, Title: "  第一章（修订）  "},
		},
	}
	detail, err := svc.SaveOutline(ctx, cmd)
	if err != nil {
		t.Fatalf("重命名保存失败: %v", err)
	}
	if got, want := detail.Tree[0].Node.Title(), "第一章（修订）"; got != want {
		t.Fatalf("保存后的标题不对: got %q want %q", got, want)
	}

	again, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("重新读取失败: %v", err)
	}
	if got, want := again.Tree[0].Node.Title(), "第一章（修订）"; got != want {
		t.Fatalf("重新打开的标题不对: got %q want %q", got, want)
	}

	// 重复提交改名后的树：不新增行、标题不回退（整树幂等）。
	cmd.Nodes = []app.OutlineInputNode{
		{ID: ctype.NewID(1), Kind: outline.NodeKindFolder, Title: "第一章（修订）"},
	}
	second, err := svc.SaveOutline(ctx, cmd)
	if err != nil {
		t.Fatalf("重复提交失败: %v", err)
	}
	if got, want := second.Tree[0].Node.Title(), "第一章（修订）"; got != want {
		t.Fatalf("重复提交后标题变了: got %q want %q", got, want)
	}
	if len(repos.FakeOutlines().AllNodes()) != 1 || repos.FakeOutlines().Calls("Create") != 0 {
		t.Fatalf("重复提交不应新增行: nodes=%d creates=%d",
			len(repos.FakeOutlines().AllNodes()), repos.FakeOutlines().Calls("Create"))
	}
}

// TestSaveOutlineIsIdempotent 断言同样内容的整树重复提交不产生变化。
func TestSaveOutlineIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)

	cmd := app.SaveOutlineCmd{
		Actor:     adminViewer(),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{
				Kind:  outline.NodeKindFolder,
				Title: "第一章",
				Children: []app.OutlineInputNode{
					{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
				},
			},
		},
	}
	first, err := svc.SaveOutline(ctx, cmd)
	if err != nil {
		t.Fatalf("第一次保存失败: %v", err)
	}
	firstIDs := collectIDs(first.Tree)

	// 第二次提交：内容相同、ID 相同（页面保存后重新读回的形态）。
	cmd.Nodes = []app.OutlineInputNode{
		{
			ID:    first.Tree[0].Node.ID(),
			Kind:  outline.NodeKindFolder,
			Title: "第一章",
			Children: []app.OutlineInputNode{
				{ID: first.Tree[0].Children[0].Node.ID(), Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
			},
		},
	}
	second, err := svc.SaveOutline(ctx, cmd)
	if err != nil {
		t.Fatalf("第二次保存失败: %v", err)
	}
	if got := collectIDs(second.Tree); !sameStrings(got, firstIDs) {
		t.Fatalf("重复提交不应产生新节点:\n first %v\nsecond %v", firstIDs, got)
	}
	if len(repos.FakeOutlines().ActiveNodes()) != 2 {
		t.Fatalf("重复提交后应仍是 2 个活跃节点，实际 %d", len(repos.FakeOutlines().ActiveNodes()))
	}
	if repos.FakeOutlines().Calls("Create") != 1 {
		t.Fatalf("第二次提交不应再插入新行，Create 调用 %d 次", repos.FakeOutlines().Calls("Create"))
	}
}

// TestAdminOutlineKeepsDeletedArticleTitleAndReason 文章被软删时后台树节点仍要
// 保留标题，并把失效原因区分为 deleted（与旧扁平 LEFT JOIN 口径一致）。
func TestAdminOutlineKeepsDeletedArticleTitleAndReason(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	seedOutlineArticle(t, repos, 11, "被删的文章", domain.StatusPublished)
	if err := repos.FakeArticles().SoftDelete(ctx, ctype.NewID(11)); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}
	repos.FakeOutlines().Seed(outlineArticle(t, 2, 0, 11, 1))

	detail, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("读回后台树失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"被删的文章"}; !sameStrings(got, want) {
		t.Fatalf("软删文章节点应保留标题:\n got %v\nwant %v", got, want)
	}
	if !detail.Tree[0].Stale() || detail.Tree[0].StaleReason() != domain.StaleReasonDeleted {
		t.Fatalf("软删文章节点的失效原因应为 %q，实际 %q",
			domain.StaleReasonDeleted, detail.Tree[0].StaleReason())
	}
}

// TestAdminOutlineMarksMissingArticleNode 节点指向的文章真不存在（被移出树 /
// 物理删除）时才给 missing，且没有标题可显示。
func TestAdminOutlineMarksMissingArticleNode(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	repos.FakeOutlines().Seed(outlineArticle(t, 2, 0, 99, 1))

	detail, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("读回后台树失败: %v", err)
	}
	if !detail.Tree[0].Stale() || detail.Tree[0].StaleReason() != domain.StaleReasonMissing {
		t.Fatalf("不存在的文章节点的失效原因应为 %q，实际 %q",
			domain.StaleReasonMissing, detail.Tree[0].StaleReason())
	}
	if detail.Tree[0].ArticleTitle != "" {
		t.Fatalf("不存在的文章节点不该有标题，实际 %q", detail.Tree[0].ArticleTitle)
	}
}

// TestSaveOutlineRetainsStaleArticleNode 断言文章后来不公开时节点保留、后台可标失效；
// 从整树提交里摘掉它则整单软删（后台「清理失效行」）。
func TestSaveOutlineRetainsStaleArticleNode(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	seedOutlineArticle(t, repos, 11, "草稿", domain.StatusDraft)
	repos.FakeOutlines().Seed(outlineArticle(t, 2, 0, 11, 1))

	detail, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID))
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if !detail.Tree[0].Stale() {
		t.Fatalf("草稿文章的节点应标为已失效")
	}

	// 整树提交时带上这个失效节点仍然是合法的（移除由运营自己决定）。
	if _, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
		Actor:     adminViewer(),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{ID: ctype.NewID(2), Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
		},
	}); err != nil {
		t.Fatalf("保留失效节点应成功: %v", err)
	}
}

// TestSaveOutlineDropsStaleArticleNode 断言提交里不再出现的失效节点被软删，
// 且重复提交空树不会反复删（幂等）。
func TestSaveOutlineDropsStaleArticleNode(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	seedOutlineArticle(t, repos, 11, "草稿", domain.StatusDraft)
	repos.FakeOutlines().Seed(
		outlineArticle(t, 2, 0, 11, 1),
		outlineFolder(t, 3, 0, "第一章", 2),
	)

	cmd := app.SaveOutlineCmd{
		Actor: adminViewer(), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{
			{ID: ctype.NewID(3), Kind: outline.NodeKindFolder, Title: "第一章"},
		},
	}
	detail, err := svc.SaveOutline(ctx, cmd)
	if err != nil {
		t.Fatalf("清理失效行失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"第一章"}; !sameStrings(got, want) {
		t.Fatalf("清理后的树不对: got %v want %v", got, want)
	}
	if len(repos.FakeOutlines().ActiveNodes()) != 1 {
		t.Fatalf("失效文章节点应软删，活跃节点 %d", len(repos.FakeOutlines().ActiveNodes()))
	}

	if _, err := svc.SaveOutline(ctx, cmd); err != nil {
		t.Fatalf("重复清理失败: %v", err)
	}
	if repos.FakeOutlines().Calls("Create") != 0 {
		t.Fatalf("清理与重复提交都不应新增行，Create 调用 %d 次", repos.FakeOutlines().Calls("Create"))
	}
}

// TestSaveOutlineRejectsInvalidInput 断言不合法输入整单拒绝且事务回滚。
func TestSaveOutlineRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	t.Run("非管理员", func(t *testing.T) {
		t.Parallel()
		svc, repos, _ := seedCollectionOutlines(t)
		ctx := context.Background()
		seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
		_, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
			Actor:     authorViewer(),
			OwnerKind: outline.OwnerCollection,
			OwnerID:   ctype.NewID(outlineOwnerID),
			Nodes:     []app.OutlineInputNode{{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)}},
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("非管理员应 403，实际 %v", err)
		}
	})

	t.Run("重复文章", func(t *testing.T) {
		t.Parallel()
		svc, repos, tx := seedCollectionOutlines(t)
		ctx := context.Background()
		seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
		_, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
			Actor:     adminViewer(),
			OwnerKind: outline.OwnerCollection,
			OwnerID:   ctype.NewID(outlineOwnerID),
			Nodes: []app.OutlineInputNode{
				{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
				{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)},
			},
		})
		if !errors.Is(err, outline.ErrDuplicateArticle) {
			t.Fatalf("重复文章应报 ErrDuplicateArticle，实际 %v", err)
		}
		if len(repos.FakeOutlines().ActiveNodes()) != 0 || tx.Rollbacks() != 1 {
			t.Fatalf("失败后不应落库且应回滚，active=%d rollbacks=%d",
				len(repos.FakeOutlines().ActiveNodes()), tx.Rollbacks())
		}
	})

	t.Run("不可收录的文章", func(t *testing.T) {
		t.Parallel()
		svc, repos, _ := seedCollectionOutlines(t)
		ctx := context.Background()
		seedOutlineArticle(t, repos, 11, "草稿", domain.StatusDraft)
		_, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
			Actor:     adminViewer(),
			OwnerKind: outline.OwnerCollection,
			OwnerID:   ctype.NewID(outlineOwnerID),
			Nodes:     []app.OutlineInputNode{{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)}},
		})
		if !errors.Is(err, outline.ErrArticleNotIncludable) {
			t.Fatalf("草稿文章应不可收录，实际 %v", err)
		}
	})

	t.Run("文章节点带子节点", func(t *testing.T) {
		t.Parallel()
		svc, repos, _ := seedCollectionOutlines(t)
		ctx := context.Background()
		seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
		_, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
			Actor:     adminViewer(),
			OwnerKind: outline.OwnerCollection,
			OwnerID:   ctype.NewID(outlineOwnerID),
			Nodes: []app.OutlineInputNode{
				{
					Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11),
					Children: []app.OutlineInputNode{{Kind: outline.NodeKindFolder, Title: "子文件夹"}},
				},
			},
		})
		if !errors.Is(err, outline.ErrStructureInvalid) {
			t.Fatalf("文章节点不能有子节点，实际 %v", err)
		}
	})

	t.Run("不存在的节点", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := seedCollectionOutlines(t)
		ctx := context.Background()
		_, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
			Actor:     adminViewer(),
			OwnerKind: outline.OwnerCollection,
			OwnerID:   ctype.NewID(outlineOwnerID),
			Nodes:     []app.OutlineInputNode{{ID: ctype.NewID(999), Kind: outline.NodeKindFolder, Title: "旧页面"}},
		})
		if !errors.Is(err, outline.ErrNodeMissing) {
			t.Fatalf("过期页面的节点应报 ErrNodeMissing，实际 %v", err)
		}
	})
}

// TestSaveOutlineReAddsRemovedArticle 断言移出后再收录沿用同一行（唯一键口径）。
func TestSaveOutlineReAddsRemovedArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)
	repos.FakeOutlines().Seed(outlineArticle(t, 2, 0, 11, 1))

	// 提交空树 → 节点软删（保留行）。
	if _, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
		Actor: adminViewer(), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(outlineOwnerID),
	}); err != nil {
		t.Fatalf("清空树失败: %v", err)
	}
	if len(repos.FakeOutlines().ActiveNodes()) != 0 {
		t.Fatalf("清空后不应有活跃节点")
	}

	// 再收录同一篇文章（不带 ID，模拟页面重建）：应恢复原行，不新增第二行。
	detail, err := svc.SaveOutline(ctx, app.SaveOutlineCmd{
		Actor: adminViewer(), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(outlineOwnerID),
		Nodes: []app.OutlineInputNode{{Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11)}},
	})
	if err != nil {
		t.Fatalf("重新收录失败: %v", err)
	}
	if got := detail.Tree[0].Node.ID(); got != ctype.NewID(2) {
		t.Fatalf("重新收录应恢复原节点 2，实际 %s", got)
	}
	if len(repos.FakeOutlines().AllNodes()) != 1 {
		t.Fatalf("重新收录不应新增行，实际 %d 行", len(repos.FakeOutlines().AllNodes()))
	}
}

// TestAdminOutlineRejectsNonAdminAndUnknownOwner 断言后台读面的门控。
func TestAdminOutlineRejectsNonAdminAndUnknownOwner(t *testing.T) {
	t.Parallel()

	svc, _, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	if _, err := svc.AdminOutline(ctx, authorViewer(), outline.OwnerCollection, ctype.NewID(outlineOwnerID)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员应 403，实际 %v", err)
	}
	if _, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerCollection, ctype.NewID(999)); !errors.Is(err, collection.ErrNotFound) {
		t.Fatalf("不存在的合集应报 ErrNotFound，实际 %v", err)
	}
}

// TestAdminOutlineSupportsProjects 断言同一套树也服务项目（owner 是项目）。
func TestAdminOutlineSupportsProjects(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()

	p, err := project.New(project.NewParams{
		ID: ctype.NewID(700), Title: "项目", CreatorID: domain.UserID(admin), Displayed: true,
		SortOrder: 1, Now: outlineNow,
	})
	if err != nil {
		t.Fatalf("构造项目失败: %v", err)
	}
	repos.FakeProjects().SeedProject(p)
	seedOutlineArticle(t, repos, 11, "第一篇", domain.StatusPublished)

	node, err := outline.New(outline.NewParams{
		ID: ctype.NewID(5), OwnerKind: outline.OwnerProject, OwnerID: ctype.NewID(700),
		Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11), SortOrder: 1, Now: outlineNow,
	})
	if err != nil {
		t.Fatalf("构造项目节点失败: %v", err)
	}
	repos.FakeOutlines().Seed(node)

	detail, err := svc.AdminOutline(ctx, adminViewer(), outline.OwnerProject, ctype.NewID(700))
	if err != nil {
		t.Fatalf("读取项目树失败: %v", err)
	}
	if got, want := viewLabels(detail.Tree), []string{"第一篇"}; !sameStrings(got, want) {
		t.Fatalf("项目树不对: %v", got)
	}
}

func collectIDs(tree []outline.ViewNode) []string {
	var out []string
	var walk func(items []outline.ViewNode)
	walk = func(items []outline.ViewNode) {
		for _, item := range items {
			out = append(out, item.Node.ID().String())
			walk(item.Children)
		}
	}
	walk(tree)
	return out
}
