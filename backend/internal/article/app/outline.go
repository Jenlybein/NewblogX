package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
)

// 本文件是两个具名树端点共用的**领域编排规则**：整树提交的展开 / 幂等复用、
// 新收录文章的可收录校验、后台读面的嵌套组装。
//
// 它不是一个对外用例：调用方必须先经 namedRepos 把事务仓储指向某张具名树表，
// 再调用 SaveNamedOutline / NamedOutlineTree / CollectionDetail / ProjectDetail。
// 旧共用表与它的 AdminOutline / SaveOutline 两个共享端点已随 #277 下线。

// OutlineInputNode 是整树提交里的一个节点。
//
// 嵌套表达父子关系（页面提交一棵树，服务端按 DFS 顺序落 parent/sort_order），
// 不引入客户端临时 ID：新节点没有 ID，服务端新建；已有节点带 ID。
type OutlineInputNode struct {
	ID        outline.ID
	Kind      outline.NodeKind
	Title     string
	ArticleID outline.ArticleID
	Children  []OutlineInputNode
}

// saveOutlineCmd 是整树提交规则的内部输入（不对外暴露旧共享命令）。
type saveOutlineCmd struct {
	OwnerKind outline.OwnerKind
	OwnerID   outline.ID
	Nodes     []OutlineInputNode
}

// plannedNode 是提交里的一行：node 是最终状态（新节点已给好 ID）。
type plannedNode struct {
	node  *outline.Node
	isNew bool
}

// planOutline 把嵌套提交展开成节点清单，并复用已有行。
//
// 复用规则（幂等的基础）：
//   - 带 ID 的节点必须属于当前树（页面过期直接拒绝，不静默丢数据）；
//   - 不带 ID 的文章节点按文章匹配已有行（含软删行，恢复而不是新插入）；
//   - 不带 ID 的文件夹按（父节点 + 标题）匹配当前活跃行。
func (s *Service) planOutline(
	ctx context.Context, r treeRepos, cmd saveOutlineCmd, current []*outline.Node, now time.Time,
) ([]*plannedNode, error) {
	byID := make(map[outline.ID]*outline.Node, len(current))
	used := make(map[outline.ID]*outline.Node, len(current))
	for _, n := range current {
		byID[n.ID()] = n
	}

	type queueItem struct {
		input    OutlineInputNode
		parentID outline.ID
	}
	queue := make([]queueItem, 0, len(cmd.Nodes))
	for _, in := range cmd.Nodes {
		queue = append(queue, queueItem{input: in})
	}
	planned := make([]*plannedNode, 0, len(cmd.Nodes))
	order := make(map[outline.ID]int, len(current))

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		in := item.input
		if !in.Kind.Valid() {
			return nil, outline.ErrStructureInvalid
		}
		if in.Kind == outline.NodeKindArticle && len(in.Children) > 0 {
			return nil, outline.ErrStructureInvalid
		}

		var existing *outline.Node
		if !in.ID.IsZero() {
			existing = byID[in.ID]
			if existing == nil {
				return nil, outline.ErrNodeMissing
			}
			if used[existing.ID()] != nil {
				return nil, outline.ErrStructureInvalid
			}
			if existing.Kind() != in.Kind {
				return nil, outline.ErrStructureInvalid
			}
			used[existing.ID()] = existing
		} else if in.Kind == outline.NodeKindFolder {
			title := strings.TrimSpace(in.Title)
			for _, n := range current {
				if used[n.ID()] != nil || !n.IsFolder() || n.ParentID() != item.parentID {
					continue
				}
				if n.Title() == title {
					existing = n
					used[n.ID()] = n
					break
				}
			}
		} else {
			for _, n := range current {
				if used[n.ID()] != nil || !n.IsArticle() {
					continue
				}
				if n.ArticleID() == in.ArticleID {
					existing = n
					used[n.ID()] = n
					break
				}
			}
			if existing == nil {
				persisted, err := r.Outlines().FindByArticle(ctx, cmd.OwnerKind, cmd.OwnerID, in.ArticleID)
				switch {
				case errors.Is(err, outline.ErrNodeMissing):
					// 这棵树里还没有这篇文章的节点，走新建。
				case err != nil:
					return nil, err
				case used[persisted.ID()] != nil:
					return nil, outline.ErrDuplicateArticle
				default:
					existing = persisted
					used[persisted.ID()] = persisted
				}
			}
		}

		order[item.parentID]++
		sortOrder := order[item.parentID]

		var node *outline.Node
		isNew := existing == nil
		if existing != nil {
			node = cloneOutlineNode(existing)
			node.MoveTo(item.parentID, sortOrder, now)
			// 标题也是整树提交的一部分：带 ID 的文件夹改名必须落到复用行上，
			// 否则页面保存成功但标题静默回退成旧值（#218）。
			if in.Kind == outline.NodeKindFolder {
				if err := node.Rename(in.Title, now); err != nil {
					return nil, err
				}
			}
			if existing.Deleted() {
				node.Restore(now)
			}
		} else {
			created, err := outline.New(outline.NewParams{
				ID:        s.ids.NextOutlineNodeID(),
				OwnerKind: cmd.OwnerKind,
				OwnerID:   cmd.OwnerID,
				ParentID:  item.parentID,
				Kind:      in.Kind,
				Title:     in.Title,
				ArticleID: in.ArticleID,
				SortOrder: sortOrder,
				Now:       now,
			})
			if err != nil {
				return nil, err
			}
			node = created
		}
		planned = append(planned, &plannedNode{node: node, isNew: isNew})

		for _, child := range in.Children {
			queue = append(queue, queueItem{input: child, parentID: node.ID()})
		}
	}
	return planned, nil
}

// checkIncludableNewArticles 只约束**新收录**：已有节点上的文章后来变成
// 私有 / 删除时节点保留（失效行口径沿用），不在这里拒绝。
func (s *Service) checkIncludableNewArticles(
	ctx context.Context, r treeRepos, planned []*plannedNode,
) error {
	for _, p := range planned {
		if !p.isNew || !p.node.IsArticle() {
			continue
		}
		a, err := r.Articles().Find(ctx, p.node.ArticleID())
		if errors.Is(err, domain.ErrArticleNotFound) {
			return outline.ErrArticleNotIncludable
		}
		if err != nil {
			return err
		}
		if !a.IsPubliclyVisible() {
			return outline.ErrArticleNotIncludable
		}
	}
	return nil
}

// buildOutlineDetail 把节点清单组装成嵌套读模型，并补齐文章节点快照。
//
// 快照用 FindIncludingDeleted（旧扁平 LEFT JOIN 的等价物）：文章被软删时节点
// 仍保留标题并标 deleted；只有节点指向的文章真的不存在时才退化成 missing。
func (s *Service) buildOutlineDetail(
	ctx context.Context, r treeRepos, ownerKind outline.OwnerKind, ownerID outline.ID, nodes []*outline.Node,
) (*outline.Detail, error) {
	tree, err := outline.Build(nodes)
	if err != nil {
		return nil, err
	}

	type snap struct {
		state *domain.ArticleState
		title string
		cover string
	}
	snaps := newSnapshotCache(func(articleID outline.ArticleID) (snap, error) {
		a, err := r.Articles().FindIncludingDeleted(ctx, articleID)
		if err != nil {
			return snap{}, err
		}
		return snap{
			state: &domain.ArticleState{
				Deleted:    a.Deleted(),
				Status:     a.Status(),
				Visibility: a.Visibility(),
			},
			title: a.Title(),
			cover: a.Cover(),
		}, nil
	})

	var attach func(items []outline.TreeNode) ([]outline.ViewNode, error)
	attach = func(items []outline.TreeNode) ([]outline.ViewNode, error) {
		out := make([]outline.ViewNode, 0, len(items))
		for _, item := range items {
			view := outline.ViewNode{Node: item.Node}
			if item.Node.IsArticle() {
				v, err := snaps.get(item.Node.ArticleID())
				if err != nil {
					return nil, err
				}
				view.Article = v.state
				view.ArticleTitle = v.title
				view.ArticleCover = v.cover
			}
			children, err := attach(item.Children)
			if err != nil {
				return nil, err
			}
			view.Children = children
			out = append(out, view)
		}
		return out, nil
	}

	treeView, err := attach(tree)
	if err != nil {
		return nil, err
	}
	return &outline.Detail{OwnerKind: ownerKind, OwnerID: ownerID, Tree: treeView}, nil
}

// checkOutlineOwner 确认树归属的实体存在（后台不能给不存在的实体建树）。
//
// 软删 / 不存在的实体返回各自子域的 ErrNotFound：后台对"合集不存在"与
// "树结构不合法"是两种提示，不该合并。
func checkOutlineOwner(
	ctx context.Context, r treeRepos, ownerKind outline.OwnerKind, ownerID outline.ID,
) error {
	switch ownerKind {
	case outline.OwnerCollection:
		_, err := r.Collections().Find(ctx, collection.ID(ownerID))
		return err
	case outline.OwnerProject:
		_, err := r.Projects().Find(ctx, project.ID(ownerID))
		return err
	default:
		return outline.ErrStructureInvalid
	}
}

// cloneOutlineNode 复制持久化节点，避免就地改仓储读出来的对象。
func cloneOutlineNode(n *outline.Node) *outline.Node {
	return outline.Rebuild(outline.RebuildParams{
		ID:        n.ID(),
		OwnerKind: n.OwnerKind(),
		OwnerID:   n.OwnerID(),
		ParentID:  n.ParentID(),
		Kind:      n.Kind(),
		Title:     n.Title(),
		ArticleID: n.ArticleID(),
		SortOrder: n.SortOrder(),
		Deleted:   n.Deleted(),
		CreatedAt: n.CreatedAt(),
		UpdatedAt: n.UpdatedAt(),
	})
}
