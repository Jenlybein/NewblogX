package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「读者侧详情读模型」的接缝（有状态 fake 仓储）：
// 详情从扁平 articles[] 换成嵌套树，只留公开文章，封面回退取深度优先
// 第一篇公开文章，未展示实体与「显示非管理员发布文章」开关的口径不变。

// readerNodeSpec 是测试里往树上播种的一行。
type readerNodeSpec struct {
	nodeID    uint64
	parentID  uint64
	kind      outline.NodeKind
	title     string
	articleID uint64
	order     int
}

// seedReaderOutline 按给定顺序往某棵树播种节点（测试准备，跳过用例层）。
func seedReaderOutline(
	t *testing.T, repos *apptest.Repos, ownerKind outline.OwnerKind, ownerID ctype.ID,
	specs ...readerNodeSpec,
) {
	t.Helper()
	nodes := make([]*outline.Node, 0, len(specs))
	for _, s := range specs {
		n, err := outline.New(outline.NewParams{
			ID:        ctype.NewID(s.nodeID),
			OwnerKind: ownerKind,
			OwnerID:   ownerID,
			ParentID:  ctype.NewID(s.parentID),
			Kind:      s.kind,
			Title:     s.title,
			ArticleID: ctype.NewID(s.articleID),
			SortOrder: s.order,
			Now:       outlineNow,
		})
		if err != nil {
			t.Fatalf("构造目录树节点失败: %v", err)
		}
		nodes = append(nodes, n)
	}
	repos.FakeOutlines().Seed(nodes...)
}

// seedReaderArticleNodes 把一组文章 ID 依次挂到根层（节点 ID 从 1000 起）。
func seedReaderArticleNodes(
	t *testing.T, repos *apptest.Repos, ownerKind outline.OwnerKind, ownerID ctype.ID,
	articleIDs ...ctype.ID,
) {
	t.Helper()
	specs := make([]readerNodeSpec, 0, len(articleIDs))
	for i, id := range articleIDs {
		specs = append(specs, readerNodeSpec{
			nodeID:    uint64(1000 + i),
			kind:      outline.NodeKindArticle,
			articleID: id.Uint64(),
			order:     i + 1,
		})
	}
	seedReaderOutline(t, repos, ownerKind, ownerID, specs...)
}

// readerDetailArticleIDs 按 DFS 顺序取读者树里的文章 ID。
func readerDetailArticleIDs(tree []outline.ViewNode) []ctype.ID {
	var out []ctype.ID
	var walk func(items []outline.ViewNode)
	walk = func(items []outline.ViewNode) {
		for _, item := range items {
			if item.Node.IsArticle() {
				out = append(out, ctype.ID(item.Node.ArticleID()))
			}
			walk(item.Children)
		}
	}
	walk(tree)
	return out
}

// readerDetailSortOrders 按 DFS 顺序取读者树里文章节点的兄弟序号。
func readerDetailSortOrders(tree []outline.ViewNode) []int {
	var out []int
	var walk func(items []outline.ViewNode)
	walk = func(items []outline.ViewNode) {
		for _, item := range items {
			if item.Node.IsArticle() {
				out = append(out, item.Node.SortOrder())
			}
			walk(item.Children)
		}
	}
	walk(tree)
	return out
}

// readerDetailLabels 把读者读模型的嵌套树拍平成「标题」序列。
func readerDetailLabels(tree []outline.ViewNode) []string {
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

// seedReaderArticles 播种三篇公开文章：第一篇与第二篇带封面，第三篇没有封面。
func seedReaderArticles(t *testing.T, repos *apptest.Repos) {
	t.Helper()
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Author(domain.UserID(admin)).
			Title("第一篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).
			Cover("https://img.example/first.png").Build(),
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(admin)).
			Title("第二篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).
			Cover("https://img.example/second.png").Build(),
		apptest.NewArticle().ID(ctype.NewID(13)).Author(domain.UserID(admin)).
			Title("第三篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).Build(),
	)
}

// TestReaderDetailReturnsNestedOutlineTree 读者详情就是目录树：文件夹保留、
// 文章节点带标题/封面快照，顺序按 DFS 稳定，篇数只数文章节点。
func TestReaderDetailReturnsNestedOutlineTree(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)

	repos.FakeOutlines().Seed(
		outlineFolder(t, 1, 0, "第一章", 1),
		outlineArticle(t, 2, 1, 11, 1),
		outlineArticle(t, 3, 1, 12, 2),
	)

	detail, err := svc.CollectionDetail(ctx, ctype.NewID(outlineOwnerID), domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if got, want := readerDetailLabels(detail.Tree), []string{"第一章", "第一篇", "第二篇"}; !sameStrings(got, want) {
		t.Fatalf("详情应是嵌套树:\n got %v\nwant %v", got, want)
	}
	if len(detail.Tree[0].Children) != 2 || !detail.Tree[0].Children[0].Node.IsArticle() {
		t.Fatalf("第一章下应有 2 个文章节点，实际 %+v", detail.Tree[0].Children)
	}
	if detail.Tree[0].Children[0].ArticleCover != "https://img.example/first.png" {
		t.Fatalf("文章节点应带封面快照，实际 %q", detail.Tree[0].Children[0].ArticleCover)
	}
	if detail.ArticleCount != 2 {
		t.Fatalf("篇数只数公开文章节点，期望 2，实际 %d", detail.ArticleCount)
	}
}

// TestReaderOutlineHidesStaleArticlesAndKeepsEmptyFolders 失效文章节点不出现在
// 读者树里，但空文件夹保留（后台可以只搭骨架，读者看到中性空态）。
func TestReaderOutlineHidesStaleArticlesAndKeepsEmptyFolders(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(admin)).
			Title("草稿篇").Status(domain.StatusDraft).Visibility(domain.VisibilityVisible).Build(),
	)
	repos.FakeOutlines().Seed(
		outlineFolder(t, 1, 0, "第一章", 1),
		outlineArticle(t, 2, 1, 11, 1),
		outlineArticle(t, 3, 1, 12, 2),
		outlineFolder(t, 4, 0, "空章节", 2),
	)

	detail, err := svc.CollectionDetail(ctx, ctype.NewID(outlineOwnerID), domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if got, want := readerDetailLabels(detail.Tree), []string{"第一章", "第一篇", "空章节"}; !sameStrings(got, want) {
		t.Fatalf("失效节点应被过滤、空文件夹保留:\n got %v\nwant %v", got, want)
	}
	if detail.ArticleCount != 1 {
		t.Fatalf("篇数只数公开文章，期望 1，实际 %d", detail.ArticleCount)
	}
}

// TestReaderOutlineCoverFallbackDepthFirst 没上传封面时回退到深度优先第一篇
// 有封面的公开文章节点；没有封面时留空（前端渲染透明空白）。
func TestReaderOutlineCoverFallbackDepthFirst(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Author(domain.UserID(admin)).
			Title("第一篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).Build(),
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(admin)).
			Title("第二篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).
			Cover("https://img.example/second.png").Build(),
	)
	repos.FakeOutlines().Seed(
		outlineArticle(t, 2, 0, 11, 1),
		outlineArticle(t, 3, 0, 12, 2),
	)

	detail, err := svc.CollectionDetail(ctx, ctype.NewID(outlineOwnerID), domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if detail.Cover != "https://img.example/second.png" {
		t.Fatalf("封面应回退到 DFS 第一篇有封面的公开文章，实际 %q", detail.Cover)
	}
}

// TestReaderOutlineExplicitCoverAndHiddenOwner 显式封面不被回退覆盖；未展示的
// 合集对读者仍按不存在处理（管理员能打开）。
func TestReaderOutlineExplicitCoverAndHiddenOwner(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	seedReaderArticles(t, repos)
	repos.FakeOutlines().Seed(outlineArticle(t, 2, 0, 11, 1))

	c := collection.Restore(collection.RestoreParams{
		ID:          ctype.NewID(outlineOwnerID),
		Title:       "专题",
		Cover:       "https://img.example/explicit.png",
		CreatorID:   domain.UserID(admin),
		Displayed:   true,
		SortOrder:   1,
		CreatedAt:   outlineNow,
		UpdatedAt:   outlineNow,
	})
	repos.FakeCollections().SeedCollection(c)
	detail, err := svc.CollectionDetail(ctx, c.ID(), domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if detail.Cover != "https://img.example/explicit.png" {
		t.Fatalf("显式封面不应被回退覆盖，实际 %q", detail.Cover)
	}

	c.SetDisplay(false, outlineNow)
	if err := repos.FakeCollections().Update(ctx, c); err != nil {
		t.Fatalf("关展示失败: %v", err)
	}
	if _, err := svc.CollectionDetail(ctx, c.ID(), domain.Anonymous(), false); !apperr.IsCode(err, apperr.CodeCollectionNotFound) {
		t.Fatalf("未展示的合集对读者应按不存在处理，实际 %v", err)
	}
	if _, err := svc.CollectionDetail(ctx, c.ID(), adminViewer(), false); err != nil {
		t.Fatalf("管理员仍应能打开未展示的合集: %v", err)
	}
}

// TestReaderOutlineHonorsShowNonAdminSwitch 「显示非管理员发布文章」关闭时，
// 非管理员作者的文章节点也不出现在读者树里。
func TestReaderOutlineHonorsShowNonAdminSwitch(t *testing.T) {
	t.Parallel()

	svc, repos, _ := seedCollectionOutlines(t)
	ctx := context.Background()
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Author(domain.UserID(admin)).
			Title("管理员篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).Build(),
		apptest.NewArticle().ID(ctype.NewID(12)).Author(domain.UserID(author)).
			Title("普通作者篇").Status(domain.StatusPublished).Visibility(domain.VisibilityVisible).Build(),
	)
	repos.FakeUsers().Seed(app.AuthorView{ID: domain.UserID(admin), Admin: true})
	repos.FakeOutlines().Seed(
		outlineArticle(t, 2, 0, 11, 1),
		outlineArticle(t, 3, 0, 12, 2),
	)

	detail, err := svc.CollectionDetail(ctx, ctype.NewID(outlineOwnerID), domain.Anonymous(), true)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if got, want := readerDetailLabels(detail.Tree), []string{"管理员篇"}; !sameStrings(got, want) {
		t.Fatalf("开关关闭时应只剩管理员作者的文章:\n got %v\nwant %v", got, want)
	}
}
