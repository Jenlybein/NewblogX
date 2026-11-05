package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
)

// 本文件是 #274 expand 期两个具名端点的用例：合集 / 项目各自读写自己的树表。
//
// 复用既有的一份领域规则（planOutline / buildOutlineDetail / readerOutline /
// articleNeighbours），只把事务内的目录树仓储换成对应实体的具名表。这不是
// "按开关二选一的兼容分支"：一次请求从路由就知道自己属于哪张表，系统对外仍是
// 两个各自完整的形状；旧共用端点继续走原来的 Repos，与新端点并存只是该票内部
// 过渡，由 #277 收缩。

// namedOutlineRepos 把一次事务内的目录树仓储换成指定实体的具名表。
//
// 嵌入 Repos 接口：其余十几个仓储照旧由合成根提供，只有 Outlines() 被覆盖。
// 这样整套领域规则不必复制一份，也不会出现"两个几乎相同的用例层"。
type namedOutlineRepos struct {
	Repos
	outlines outline.Repository
}

// Outlines 返回该实体自己的具名树表仓储。
func (r namedOutlineRepos) Outlines() outline.Repository { return r.outlines }

// NamedOutlineRepository 由使用方定义：按实体取出自己的具名树表仓储。
// 端口只暴露 1 个方法，实现方（adapter/mysql）不得反向定义。实现就在
// 事务内的 Repos 上，因此具名仓储与其余仓储共用同一个事务句柄。
type NamedOutlineRepository interface {
	NamedOutlineFor(ctx context.Context, kind outline.OwnerKind) outline.Repository
}

// NamedOutlineRepos 把一次事务的 Repos 收成「指向某张具名树表」的 Repos。
//
// Repos 不满足 NamedOutlineRepository 时会返回错误而不是悄悄退回旧共用表：
// 那会让具名端点写错表却全绿，正是零 DDL 拆树最危险的失效形态。
func namedRepos(ctx context.Context, repos Repos, kind outline.OwnerKind) (Repos, error) {
	nr, ok := repos.(NamedOutlineRepository)
	if !ok {
		return nil, outline.ErrStructureInvalid
	}
	return namedOutlineRepos{Repos: repos, outlines: nr.NamedOutlineFor(ctx, kind)}, nil
}

// SaveNamedOutlineCmd 是具名端点整树提交的输入。
type SaveNamedOutlineCmd struct {
	Actor     domain.Viewer
	OwnerKind outline.OwnerKind
	OwnerID   outline.ID
	Nodes     []OutlineInputNode
}

// SaveNamedOutline 整棵树一次性提交到该实体的具名表（管理员）。
func (s *Service) SaveNamedOutline(ctx context.Context, cmd SaveNamedOutlineCmd) (*outline.Detail, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !cmd.OwnerKind.Valid() || cmd.OwnerID.IsZero() {
		return nil, outline.ErrStructureInvalid
	}

	now := s.clock.Now()
	var detail *outline.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		r, err := namedRepos(ctx, repos, cmd.OwnerKind)
		if err != nil {
			return err
		}
		if err := checkOutlineOwner(ctx, r, cmd.OwnerKind, cmd.OwnerID); err != nil {
			return err
		}
		current, err := r.Outlines().ListNodes(ctx, cmd.OwnerKind, cmd.OwnerID)
		if err != nil {
			return err
		}

		planned, err := s.planOutline(ctx, r, SaveOutlineCmd{
			OwnerKind: cmd.OwnerKind,
			OwnerID:   cmd.OwnerID,
			Nodes:     cmd.Nodes,
		}, current, now)
		if err != nil {
			return err
		}

		nodes := make([]*outline.Node, 0, len(planned))
		for _, p := range planned {
			nodes = append(nodes, p.node)
		}
		if err := outline.Validate(nodes); err != nil {
			return err
		}
		if err := s.checkIncludableNewArticles(ctx, r, planned); err != nil {
			return err
		}

		var creates, updates []*outline.Node
		used := make(map[outline.ID]bool, len(current))
		for _, p := range planned {
			if p.isNew {
				creates = append(creates, p.node)
				continue
			}
			updates = append(updates, p.node)
			used[p.node.ID()] = true
		}
		if len(creates) > 0 {
			if err := r.Outlines().Create(ctx, creates); err != nil {
				return err
			}
		}
		if len(updates) > 0 {
			if err := r.Outlines().Update(ctx, updates); err != nil {
				return err
			}
		}
		var removed []outline.ID
		for _, n := range current {
			if !used[n.ID()] {
				removed = append(removed, n.ID())
			}
		}
		if len(removed) > 0 {
			if err := r.Outlines().SoftDelete(ctx, removed, now); err != nil {
				return err
			}
		}

		detail, err = s.buildOutlineDetail(ctx, r, cmd.OwnerKind, cmd.OwnerID, nodes)
		return err
	})
	return detail, err
}

// NamedOutlineView 是具名端点的读者读模型：一棵树 + 篇数 / 封面回退 +
// 当前 / 上一章 / 下一章。字段名与响应 DTO 一一对应。
type NamedOutlineView struct {
	HasTree      bool
	OwnerKind    outline.OwnerKind
	OwnerID      outline.ID
	Tree         []outline.ViewNode
	ArticleCount int
	Cover        string
	Current      *outline.ViewNode
	Previous     *outline.ViewNode
	Next         *outline.ViewNode
}

// NamedOutlineTree 读某实体具名树（公开读；管理员同一端点拿后台读面）。
//
// `from` 是阅读页带来的 `<kind>:<id>` 来路：给了就必须指向这一棵树，否则按
// 没有可展示的树处理（不串到别的专题）。`articleID` 为空时不回答相邻文章。
// 归属 kind / ID 由端点身份给出，响应里带回它们，读者侧据此生成 `?from=` 链接。
//
// 管理员是**有意例外**（与 CollectionDetail / ProjectDetail 同一口径）：后台树
// 编辑器要能打开未展示的实体，且必须读回含失效行的整棵树，否则一次保存会把
// 失效行与未展示树软删掉。管理员读面的 HasTree 按后台节点算，篇数 / 封面 /
// 相邻文章留零值 / null（它们只对读者读面有意义）；匿名读者路径逐字不变。
func (s *Service) NamedOutlineTree(
	ctx context.Context, kind outline.OwnerKind, ownerID outline.ID,
	from string, articleID domain.ArticleID, onlyAdminAuthors bool, viewer domain.Viewer,
) (*NamedOutlineView, error) {
	var view *NamedOutlineView
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		r, err := namedRepos(ctx, repos, kind)
		if err != nil {
			return err
		}
		view = &NamedOutlineView{OwnerKind: kind, OwnerID: ownerID}

		if from != "" {
			src, ok := ParseArticleOutlineSource(from)
			if !ok || src.OwnerKind != kind || src.OwnerID != ownerID {
				return nil
			}
		}
		if !viewer.IsAdmin() {
			visible, err := readerOutlineOwnerVisible(ctx, r, ArticleOutlineSource{
				OwnerKind: kind, OwnerID: ownerID,
			})
			if err != nil {
				return err
			}
			if !visible {
				return nil
			}
		}

		if viewer.IsAdmin() {
			// 后台读面：buildOutlineDetail 保留失效行与 stale_reason，不按读者
			// 可见性过滤，也不算篇数 / 封面 / 相邻文章。
			nodes, err := r.Outlines().ListNodes(ctx, kind, ownerID)
			if err != nil {
				return err
			}
			detail, err := s.buildOutlineDetail(ctx, r, kind, ownerID, nodes)
			if err != nil {
				return err
			}
			view.Tree = detail.Tree
			view.HasTree = len(detail.Tree) > 0
			return nil
		}

		tree, err := s.readerOutline(ctx, r, kind, ownerID, onlyAdminAuthors, "")
		if err != nil {
			return err
		}
		view.Tree = tree.Tree
		view.ArticleCount = tree.ArticleCount
		view.Cover = tree.Cover
		view.HasTree = len(tree.Tree) > 0

		if articleID.IsZero() {
			return nil
		}
		view.Current, view.Previous, view.Next = articleNeighbours(
			tree.Tree, outline.ArticleID(articleID))
		return nil
	})
	return view, err
}
