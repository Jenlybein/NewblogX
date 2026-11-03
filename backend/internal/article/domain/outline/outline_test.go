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

// TestMoveIndentAndOutdent 钉住"缩进 / 移出"的层级与顺序。
func TestMoveIndentAndOutdent(t *testing.T) {
	t.Parallel()

	// 初始：
	//   1 A:11
	//   2 A:12
	//   3 A:13
	nodes := []*outline.Node{
		articleNode(t, 1, 0, 11, 1),
		articleNode(t, 2, 0, 12, 2),
		articleNode(t, 3, 0, 13, 3),
	}

	// 先把 2 缩进到一个新文件夹下：新建文件夹作为兄长的前置条件不成立
	// （前面的兄弟是文章），因此先放一个文件夹排第一。
	nodes = append(nodes, folder(t, 4, 0, "第一章", 0))
	// 重新编号：第一章 4(0), A11 1(1), A12 2(2), A13 3(3)
	// 缩进 A12 → 挂到第一章（它前一个兄弟是 A11？不是，是 A11 在第一章之后）。
	// 直接把 A12 缩进到"前一个兄弟" A11 会失败（文章不能做父），这是规则的一部分。
	if _, err := outline.Move(nodes, ctype.NewID(2), outline.MoveIndent); !errors.Is(err, outline.ErrStructureInvalid) {
		t.Fatalf("缩进到文章节点下应报结构错误，实际 %v", err)
	}

	// 正常缩进：让 A13 缩进到第一章。需先让第一章成为 A13 的直接前一个兄弟：
	// 初始顺序第一章, A11, A12, A13 —— 前一个兄弟是 A12，所以先把它挪回第一章下？
	// 更直接：构造一章 + 文章 + 文章，把第二篇缩进到文件夹。
	nodes = []*outline.Node{
		folder(t, 4, 0, "第一章", 1),
		articleNode(t, 1, 0, 11, 2),
		articleNode(t, 2, 0, 12, 3),
	}
	// 先把 A11 缩进第一章（前一个兄弟是文件夹）。
	moved, err := outline.Move(nodes, ctype.NewID(1), outline.MoveIndent)
	if err != nil {
		t.Fatalf("缩进失败: %v", err)
	}
	if moved[0].Kind() != outline.NodeKindFolder || moved[1].ParentID() != moved[0].ID() {
		t.Fatalf("缩进后 A11 应成为第一章的子节点: %+v", moved)
	}
	if moved[1].SortOrder() != 1 {
		t.Fatalf("缩进后应是第一个子节点，sort_order=%d", moved[1].SortOrder())
	}

	// 再把 A12 缩进第一章：此时第一章的直接子节点已有一个 A11，A12 排在 A11 后。
	moved2, err := outline.Move(moved, ctype.NewID(2), outline.MoveIndent)
	if err != nil {
		t.Fatalf("第二次缩进失败: %v", err)
	}
	if got := flatLabels(mustBuild(t, moved2)); !equal(got, []string{"第一章", "A:11", "A:12"}) {
		t.Fatalf("两次缩进后的树不对: %v", got)
	}

	// 移出 A12 → 回到根层，排在第一章之后（作为第一章的下一个兄弟）。
	moved3, err := outline.Move(moved2, ctype.NewID(2), outline.MoveOutdent)
	if err != nil {
		t.Fatalf("移出失败: %v", err)
	}
	want := []string{"第一章", "A:11", "A:12"}
	if got := flatLabels(mustBuild(t, moved3)); !equal(got, want) {
		t.Fatalf("移出后的树不对: %v", got)
	}
	// A12 回到根层。
	for _, n := range moved3 {
		if n.ID() == ctype.NewID(2) && n.ParentID() != "" {
			t.Fatalf("移出后 A12 应回到根层，实际 parent=%s", n.ParentID())
		}
	}
}

// TestMoveUpAndDownSwapSiblingOrder 钉住上移 / 下移只改兄弟顺序，不改层级。
func TestMoveUpAndDownSwapSiblingOrder(t *testing.T) {
	t.Parallel()

	nodes := []*outline.Node{
		articleNode(t, 1, 0, 11, 1),
		articleNode(t, 2, 0, 12, 2),
		articleNode(t, 3, 0, 13, 3),
	}

	moved, err := outline.Move(nodes, ctype.NewID(2), outline.MoveUp)
	if err != nil {
		t.Fatalf("上移失败: %v", err)
	}
	if got := flatLabels(mustBuild(t, moved)); !equal(got, []string{"A:12", "A:11", "A:13"}) {
		t.Fatalf("上移后的顺序不对: %v", got)
	}

	moved2, err := outline.Move(moved, ctype.NewID(2), outline.MoveDown)
	if err != nil {
		t.Fatalf("下移失败: %v", err)
	}
	if got := flatLabels(mustBuild(t, moved2)); !equal(got, []string{"A:11", "A:12", "A:13"}) {
		t.Fatalf("下移回来的顺序不对: %v", got)
	}

	if _, err := outline.Move(nodes, ctype.NewID(1), outline.MoveUp); !errors.Is(err, outline.ErrStructureInvalid) {
		t.Fatalf("第一个再上移应报结构错误，实际 %v", err)
	}
	if _, err := outline.Move(nodes, ctype.NewID(3), outline.MoveDown); !errors.Is(err, outline.ErrStructureInvalid) {
		t.Fatalf("最后一个再下移应报结构错误，实际 %v", err)
	}
	if _, err := outline.Move(nodes, ctype.NewID(99), outline.MoveUp); !errors.Is(err, outline.ErrNodeMissing) {
		t.Fatalf("不存在的节点应报 ErrNodeMissing，实际 %v", err)
	}
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
