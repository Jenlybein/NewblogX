package outline_test

import (
	"errors"
	"testing"

	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「读者视图」领域纯函数的契约：详情页与列表卡片必须共用一份遍历，
// 否则封面回退顺序会在两处各走一套。
//
// 断言的是外部行为：过滤后还剩哪些节点、篇数、DFS 第一篇公开文章节点的封面。

// articleSnapshot 造一份"公开且带标题/封面"的快照。
func articleSnapshot(title, cover string) outline.ReaderSnapshot {
	return outline.ReaderSnapshot{Visible: true, Title: title, Cover: cover}
}

// readerViewLabels 把读者视图拍平成「标题」序列（文章节点用快照标题）。
func readerViewLabels(tree []outline.ViewNode) []string {
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

// TestReaderViewFiltersInvisibleAndKeepsEmptyFolders 失效文章节点被移除，
// 文件夹（含空文件夹）保留；篇数只数保留下来的文章节点。
func TestReaderViewFiltersInvisibleAndKeepsEmptyFolders(t *testing.T) {
	t.Parallel()

	tree, err := outline.Build([]*outline.Node{
		folder(t, 1, 0, "第一章", 1),
		articleNode(t, 2, 1, 11, 1),
		articleNode(t, 3, 1, 12, 2),
		folder(t, 4, 0, "空章节", 2),
	})
	if err != nil {
		t.Fatalf("组装树失败: %v", err)
	}

	snapshots := map[ctype.ID]outline.ReaderSnapshot{
		ctype.NewID(11): articleSnapshot("第一篇", "https://img.example/first.png"),
		ctype.NewID(12): {Visible: false, Title: "草稿篇"},
	}
	view, err := outline.ReaderView(tree, func(id outline.ArticleID) (outline.ReaderSnapshot, error) {
		return snapshots[ctype.ID(id)], nil
	})
	if err != nil {
		t.Fatalf("组装读者视图失败: %v", err)
	}

	if got, want := readerViewLabels(view.Tree), []string{"第一章", "第一篇", "空章节"}; !equal(got, want) {
		t.Fatalf("读者视图不对:\n got %v\nwant %v", got, want)
	}
	if view.ArticleCount != 1 {
		t.Fatalf("篇数只数公开文章节点，期望 1，实际 %d", view.ArticleCount)
	}
}

// TestReaderViewCoverIsFirstPublicArticleNode 封面取深度优先**第一个公开
// 文章节点**的封面，失效节点不参与。
func TestReaderViewCoverIsFirstPublicArticleNode(t *testing.T) {
	t.Parallel()

	tree, err := outline.Build([]*outline.Node{
		folder(t, 1, 0, "第一章", 1),
		articleNode(t, 2, 1, 11, 1),
		articleNode(t, 3, 1, 12, 2),
		articleNode(t, 4, 0, 13, 2),
		articleNode(t, 5, 0, 14, 3),
	})
	if err != nil {
		t.Fatalf("组装树失败: %v", err)
	}

	snapshots := map[ctype.ID]outline.ReaderSnapshot{
		ctype.NewID(11): articleSnapshot("第一篇", "https://img.example/first.png"),
		ctype.NewID(12): articleSnapshot("第二篇", "https://img.example/second.png"),
		ctype.NewID(13): articleSnapshot("第三篇", "https://img.example/third.png"),
		ctype.NewID(14): {Visible: false, Title: "已失效", Cover: "https://img.example/deleted.png"},
	}
	view, err := outline.ReaderView(tree, func(id outline.ArticleID) (outline.ReaderSnapshot, error) {
		return snapshots[ctype.ID(id)], nil
	})
	if err != nil {
		t.Fatalf("组装读者视图失败: %v", err)
	}

	if view.Cover != "https://img.example/first.png" {
		t.Fatalf("封面应是 DFS 第一个公开文章节点的封面，实际 %q", view.Cover)
	}
	if view.ArticleCount != 3 {
		t.Fatalf("篇数应只数公开文章，期望 3，实际 %d", view.ArticleCount)
	}
}

// TestReaderViewCoverDoesNotBorrowFromLaterArticle 第一个公开文章节点没有
// 封面时，最终封面就是空（卡片交给 #210 的空白兜底），不能借用后一篇的封面。
func TestReaderViewCoverDoesNotBorrowFromLaterArticle(t *testing.T) {
	t.Parallel()

	tree, err := outline.Build([]*outline.Node{
		articleNode(t, 1, 0, 11, 1), // 第一个公开文章节点，没有封面
		articleNode(t, 2, 0, 12, 2), // 后面这篇有封面
	})
	if err != nil {
		t.Fatalf("组装树失败: %v", err)
	}

	snapshots := map[ctype.ID]outline.ReaderSnapshot{
		ctype.NewID(11): articleSnapshot("第一篇", ""),
		ctype.NewID(12): articleSnapshot("第二篇", "https://img.example/second.png"),
	}
	view, err := outline.ReaderView(tree, func(id outline.ArticleID) (outline.ReaderSnapshot, error) {
		return snapshots[ctype.ID(id)], nil
	})
	if err != nil {
		t.Fatalf("组装读者视图失败: %v", err)
	}
	if view.Cover != "" {
		t.Fatalf("首篇公开文章没有封面时不应借用后一篇，实际 %q", view.Cover)
	}
}

// TestReaderViewPropagatesSnapshotError 快照读取失败必须向上传递，
// 不能静默把文章当成失效而让页面少显示一篇。
func TestReaderViewPropagatesSnapshotError(t *testing.T) {
	t.Parallel()

	boom := errors.New("读文章失败")
	tree, err := outline.Build([]*outline.Node{articleNode(t, 1, 0, 11, 1)})
	if err != nil {
		t.Fatalf("组装树失败: %v", err)
	}
	if _, err := outline.ReaderView(tree, func(outline.ArticleID) (outline.ReaderSnapshot, error) {
		return outline.ReaderSnapshot{}, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("快照错误应原样上抛，实际 %v", err)
	}
}
