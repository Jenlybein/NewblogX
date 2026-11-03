package outline_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是目录树**领域纯函数**的契约：嵌套合法性、同树内文章唯一、
// 缩进/移出后的层级与顺序。都在最高、最便宜的接缝上断言，不认识 SQL / gin。

var baseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func folder(t *testing.T, id, parentID uint64, title string, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(900),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindFolder,
		Title:     title,
		SortOrder: order,
		Now:       baseTime,
	})
	if err != nil {
		t.Fatalf("构造文件夹 %q 失败: %v", title, err)
	}
	return n
}

func articleNode(t *testing.T, id, parentID, articleID uint64, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(900),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindArticle,
		ArticleID: ctype.NewID(articleID),
		SortOrder: order,
		Now:       baseTime,
	})
	if err != nil {
		t.Fatalf("构造文章节点 %d 失败: %v", articleID, err)
	}
	return n
}

// flatLabels 把嵌套树拍成 "缩进标题" 列表，便于一眼看出层级与顺序。
func flatLabels(nodes []outline.TreeNode) []string {
	var out []string
	var walk func(items []outline.TreeNode, depth int)
	walk = func(items []outline.TreeNode, depth int) {
		for _, item := range items {
			label := item.Node.Title()
			if item.Node.Kind() == outline.NodeKindArticle {
				label = "A:" + item.Node.ArticleID().String()
			}
			out = append(out, label)
			walk(item.Children, depth+1)
		}
	}
	walk(nodes, 0)
	return out
}

// TestBuildTreeNestsFoldersAndArticlesInOrder 是后台读回"同一棵嵌套树"的领域契约。
func TestBuildTreeNestsFoldersAndArticlesInOrder(t *testing.T) {
	t.Parallel()

	nodes := []*outline.Node{
		articleNode(t, 1, 0, 11, 2),
		folder(t, 2, 0, "第一章", 1),
		articleNode(t, 3, 2, 12, 1),
		folder(t, 4, 2, "空章节", 2),
		articleNode(t, 5, 0, 13, 3),
	}

	tree, err := outline.Build(nodes)
	if err != nil {
		t.Fatalf("合法树不应报错: %v", err)
	}
	if got, want := flatLabels(tree), []string{"第一章", "A:12", "空章节", "A:11", "A:13"}; !equal(got, want) {
		t.Fatalf("嵌套顺序不对:\n got %v\nwant %v", got, want)
	}

	// 空文件夹必须保留：它表达"先搭章节骨架"。
	if len(tree[0].Children) != 2 {
		t.Fatalf("第一章应有 2 个子节点，实际 %d", len(tree[0].Children))
	}
	if tree[0].Children[1].Node.Kind() != outline.NodeKindFolder || len(tree[0].Children[1].Children) != 0 {
		t.Fatalf("空文件夹丢了：%+v", tree[0].Children[1])
	}
}

func TestBuildTreeRejectsDuplicateArticleInSameTree(t *testing.T) {
	t.Parallel()

	nodes := []*outline.Node{
		articleNode(t, 1, 0, 11, 1),
		articleNode(t, 2, 0, 11, 2),
	}
	if _, err := outline.Build(nodes); !errors.Is(err, outline.ErrDuplicateArticle) {
		t.Fatalf("同树内重复文章应报 ErrDuplicateArticle，实际 %v", err)
	}
}

func TestBuildTreeRejectsCycleAndArticleAsParent(t *testing.T) {
	t.Parallel()

	t.Run("自引用", func(t *testing.T) {
		t.Parallel()
		n := folder(t, 1, 1, "自己挂自己", 1)
		if _, err := outline.Build([]*outline.Node{n}); !errors.Is(err, outline.ErrStructureInvalid) {
			t.Fatalf("自引用应报结构错误，实际 %v", err)
		}
	})

	t.Run("两个文件夹成环", func(t *testing.T) {
		t.Parallel()
		a := folder(t, 1, 2, "A", 1)
		b := folder(t, 2, 1, "B", 1)
		if _, err := outline.Build([]*outline.Node{a, b}); !errors.Is(err, outline.ErrStructureInvalid) {
			t.Fatalf("成环应报结构错误，实际 %v", err)
		}
	})

	t.Run("文章节点不能有子节点", func(t *testing.T) {
		t.Parallel()
		a := articleNode(t, 1, 0, 11, 1)
		child := folder(t, 2, 1, "文章下面挂文件夹", 1)
		if _, err := outline.Build([]*outline.Node{a, child}); !errors.Is(err, outline.ErrStructureInvalid) {
			t.Fatalf("文章做父节点应报结构错误，实际 %v", err)
		}
	})

	t.Run("父节点不在树里", func(t *testing.T) {
		t.Parallel()
		n := folder(t, 1, 99, "孤儿", 1)
		if _, err := outline.Build([]*outline.Node{n}); !errors.Is(err, outline.ErrStructureInvalid) {
			t.Fatalf("缺父节点应报结构错误，实际 %v", err)
		}
	})
}

func TestValidateOwnerAndTitleRules(t *testing.T) {
	t.Parallel()

	collectionNode := folder(t, 1, 0, "章节", 1)
	owner, _ := outline.New(outline.NewParams{
		ID: ctype.NewID(2), OwnerKind: outline.OwnerProject, OwnerID: ctype.NewID(900),
		Kind: outline.NodeKindFolder, Title: "另一个 owner", SortOrder: 1, Now: baseTime,
	})
	if err := outline.Validate([]*outline.Node{collectionNode, owner}); !errors.Is(err, outline.ErrStructureInvalid) {
		t.Fatalf("混入不同 owner 的节点应报结构错误，实际 %v", err)
	}

	t.Run("文件夹标题必填", func(t *testing.T) {
		t.Parallel()
		_, err := outline.New(outline.NewParams{
			ID: ctype.NewID(3), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(900),
			Kind: outline.NodeKindFolder, Title: "   ", Now: baseTime,
		})
		if !errors.Is(err, outline.ErrTitleRequired) {
			t.Fatalf("空标题应报 ErrTitleRequired，实际 %v", err)
		}
	})

	t.Run("文件夹标题不超长", func(t *testing.T) {
		t.Parallel()
		long := make([]rune, outline.MaxTitleLen+1)
		for i := range long {
			long[i] = '长'
		}
		_, err := outline.New(outline.NewParams{
			ID: ctype.NewID(4), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(900),
			Kind: outline.NodeKindFolder, Title: string(long), Now: baseTime,
		})
		if !errors.Is(err, outline.ErrTitleTooLong) {
			t.Fatalf("超长标题应报 ErrTitleTooLong，实际 %v", err)
		}
	})
}

func mustBuild(t *testing.T, nodes []*outline.Node) []outline.TreeNode {
	t.Helper()
	tree, err := outline.Build(nodes)
	if err != nil {
		t.Fatalf("构造树失败: %v", err)
	}
	return tree
}

func equal(a, b []string) bool {
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
