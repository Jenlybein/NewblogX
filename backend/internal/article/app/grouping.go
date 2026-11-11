package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件是合集 / 项目分组的用例（ADR-0041）。
//
// 两个实体的分组语义完全一致，差异只在仓储指向哪一对表，所以用例只写一份：
// 调用方给出 OwnerKind，由 repos 决定用哪份 GroupRepository。
//
// 三条口径（与前端交互一一对应）：
//  1. 分组 CRUD **立即生效**：新建 / 改名 / 删除各是一次提交，不参与拖拽的
//     10 秒防抖 —— 用户点了"删除分组"却要等 10 秒才生效是不可接受的；
//  2. 整树换序是**幂等整份覆盖**：提交的一定是界面上的最终排列；
//  3. 删分组只删编排，条目退回未分组，**不删合集 / 项目本身**。

// GroupRepositoryFor 由使用方定义：按实体取出自己的分组仓储。
//
// 端口只暴露 1 个方法，实现方（adapter/mysql）不得反向定义；实现就在事务内的
// Repos 上，因此分组仓储与其余仓储共用同一个事务句柄。
type GroupRepositoryFor interface {
	GroupsFor(ctx context.Context, kind outline.OwnerKind) grouping.Repository
}

// groupRepos 把一次事务的 Repos 收成"指向某个实体分组表"的视图。
//
// Repos 不满足 GroupRepositoryFor 时返回错误而不是悄悄用另一张表：
// 那会让项目分组写到合集表里却全绿。
func groupRepos(ctx context.Context, repos Repos, kind outline.OwnerKind) (grouping.Repository, error) {
	gr, ok := repos.(GroupRepositoryFor)
	if !ok {
		return nil, grouping.ErrNotFound
	}
	return gr.GroupsFor(ctx, kind), nil
}

// ── 分组 CRUD ─────────────────────────────────────────────────

// SaveGroupCmd 是新建 / 改名分组的输入（ID 为零值表示新建）。
type SaveGroupCmd struct {
	Actor     domain.Viewer
	OwnerKind outline.OwnerKind
	ID        grouping.ID
	Title     string
}

// SaveGroup 新建或改名一个分组（管理员，立即生效）。
//
// 新建时排在末尾：顺序号 = 当前最大 + 1。改名不动顺序，也不动条目归属。
func (s *Service) SaveGroup(ctx context.Context, cmd SaveGroupCmd) (*grouping.Group, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !cmd.OwnerKind.Valid() {
		return nil, grouping.ErrOrderMismatch
	}

	var saved *grouping.Group
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		repo, err := groupRepos(ctx, repos, cmd.OwnerKind)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		if cmd.ID.IsZero() {
			groups, err := repo.ListGroups(ctx)
			if err != nil {
				return err
			}
			next := 1
			for _, g := range groups {
				if g.SortOrder() >= next {
					next = g.SortOrder() + 1
				}
			}
			g, err := grouping.New(grouping.NewParams{
				ID:        s.ids.NextGroupID(),
				Title:     cmd.Title,
				SortOrder: next,
				Now:       now,
			})
			if err != nil {
				return err
			}
			if err := repo.SaveGroup(ctx, g); err != nil {
				return err
			}
			saved = g
			return nil
		}

		g, err := repo.FindGroup(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := g.Rename(cmd.Title, now); err != nil {
			return err
		}
		if err := repo.SaveGroup(ctx, g); err != nil {
			return err
		}
		saved = g
		return nil
	})
	return saved, err
}

// DeleteGroupCmd 是删除分组的输入。
type DeleteGroupCmd struct {
	Actor     domain.Viewer
	OwnerKind outline.OwnerKind
	ID        grouping.ID
}

// DeleteGroup 删除一个分组（管理员，立即生效）。
//
// 只删编排：分组下的条目退回未分组，合集 / 项目本身一行不动。
func (s *Service) DeleteGroup(ctx context.Context, cmd DeleteGroupCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}
	if !cmd.OwnerKind.Valid() || cmd.ID.IsZero() {
		return grouping.ErrNotFound
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		repo, err := groupRepos(ctx, repos, cmd.OwnerKind)
		if err != nil {
			return err
		}
		return repo.DeleteGroup(ctx, cmd.ID)
	})
}

// GroupTree 是后台分组读面：分组（含组内条目顺序）+ 未分组条目顺序。
type GroupTree struct {
	Groups    []GroupNode
	Ungrouped []ctype.ID
}

// GroupNode 是一个分组与它的条目顺序。
type GroupNode struct {
	ID    grouping.ID
	Title string
	Items []ctype.ID
}

// LoadGroupTree 读出后台分组读面（管理员）。
//
// 未分组的条目由"全部条目 − 已分组条目"算出，而不是再查一次：
// 条目表里 group_id 为零值的行与"还没有条目行的实体"都算未分组，
// 前者有行、后者没有 —— 只查表会把从没进过分组的实体漏掉。
func (s *Service) LoadGroupTree(ctx context.Context, actor domain.Viewer, kind outline.OwnerKind) (*GroupTree, error) {
	if !actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !kind.Valid() {
		return nil, grouping.ErrOrderMismatch
	}

	var tree *GroupTree
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		repo, err := groupRepos(ctx, repos, kind)
		if err != nil {
			return err
		}
		groups, err := repo.ListGroups(ctx)
		if err != nil {
			return err
		}
		assignments, err := repo.ListAssignments(ctx)
		if err != nil {
			return err
		}

		byGroup := make(map[grouping.ID][]ctype.ID, len(groups))
		ungrouped := make([]ctype.ID, 0, len(assignments))
		for _, a := range assignments {
			if a.GroupID.IsZero() {
				ungrouped = append(ungrouped, a.OwnerID)
				continue
			}
			byGroup[a.GroupID] = append(byGroup[a.GroupID], a.OwnerID)
		}

		out := &GroupTree{Groups: make([]GroupNode, 0, len(groups)), Ungrouped: ungrouped}
		for _, g := range groups {
			out.Groups = append(out.Groups, GroupNode{
				ID:    g.ID(),
				Title: g.Title(),
				Items: byGroup[g.ID()],
			})
		}
		tree = out
		return nil
	})
	return tree, err
}

// ReplaceGroupOrderCmd 是整树换序的输入。
type ReplaceGroupOrderCmd struct {
	Actor     domain.Viewer
	OwnerKind outline.OwnerKind
	Tree      grouping.TreeOrder
}

// ReplaceGroupOrder 整份覆盖分组顺序与条目归属（管理员）。
//
// 集合一致性先校验再落库：后台拿到的是上一次读到的列表，别人刚建或刚删一个
// 分组后，宽松的"缺几个也照写"会把那条静默排丢。校验用当前库里的全集，
// 所以并发新建 / 删除会整单拒绝，前端刷新后重排。
func (s *Service) ReplaceGroupOrder(ctx context.Context, cmd ReplaceGroupOrderCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}
	if !cmd.OwnerKind.Valid() {
		return grouping.ErrOrderMismatch
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		repo, err := groupRepos(ctx, repos, cmd.OwnerKind)
		if err != nil {
			return err
		}
		groups, err := repo.ListGroups(ctx)
		if err != nil {
			return err
		}
		assignments, err := repo.ListAssignments(ctx)
		if err != nil {
			return err
		}

		groupIDs := make([]grouping.ID, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID())
		}
		owners := make([]ctype.ID, 0, len(assignments))
		for _, a := range assignments {
			owners = append(owners, a.OwnerID)
		}
		if err := grouping.CheckOrder(groupIDs, owners, cmd.Tree); err != nil {
			return err
		}
		return repo.ReplaceOrder(ctx, cmd.Tree)
	})
}

// ── compose：字段 + 目录树一次提交 ──────────────────────────────

// ComposeCollectionCmd 是合集"字段 + 目录树"的一次原子提交。
type ComposeCollectionCmd struct {
	Actor       domain.Viewer
	ID          collection.ID
	Title       string
	Description string
	Cover       string
	Nodes       []OutlineInputNode
}

// ComposeCollection 在一个事务里写合集字段与整棵目录树（管理员）。
//
// 为什么合成一个端点：编辑弹窗里"改标题 + 拖完树"是一次保存动作。分成两个
// 请求，任何一个失败都会留下"标题改了但树没改"的半截状态，而用户只点了一次
// 保存 —— 他没有任何线索知道该重试哪一个。
func (s *Service) ComposeCollection(ctx context.Context, cmd ComposeCollectionCmd) (*outline.Detail, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	now := s.clock.Now()
	var detail *outline.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		c, err := repos.Collections().Find(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := c.Retitle(cmd.Title, now); err != nil {
			return err
		}
		c.SetDescription(cmd.Description, now)
		c.SetCover(cmd.Cover, now)
		if err := repos.Collections().Update(ctx, c); err != nil {
			return err
		}

		built, err := s.saveNamedOutlineInTx(ctx, repos, SaveNamedOutlineCmd{
			Actor:     cmd.Actor,
			OwnerKind: outline.OwnerCollection,
			OwnerID:   outline.ID(cmd.ID),
			Nodes:     cmd.Nodes,
		}, now)
		if err != nil {
			return err
		}
		detail = built
		return nil
	})
	return detail, err
}

// ComposeProjectCmd 是项目"字段 + 目录树"的一次原子提交。
type ComposeProjectCmd struct {
	Actor       domain.Viewer
	ID          project.ID
	Title       string
	Description string
	Cover       string
	Nodes       []OutlineInputNode
}

// ComposeProject 在一个事务里写项目字段与整棵目录树（管理员）。
//
// 与 ComposeCollection 同构但不共用数据：一篇文章可以同时进两棵树。
func (s *Service) ComposeProject(ctx context.Context, cmd ComposeProjectCmd) (*outline.Detail, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	now := s.clock.Now()
	var detail *outline.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		p, err := repos.Projects().Find(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := p.Retitle(cmd.Title, now); err != nil {
			return err
		}
		p.SetDescription(cmd.Description, now)
		p.SetCover(cmd.Cover, now)
		if err := repos.Projects().Update(ctx, p); err != nil {
			return err
		}

		built, err := s.saveNamedOutlineInTx(ctx, repos, SaveNamedOutlineCmd{
			Actor:     cmd.Actor,
			OwnerKind: outline.OwnerProject,
			OwnerID:   outline.ID(cmd.ID),
			Nodes:     cmd.Nodes,
		}, now)
		if err != nil {
			return err
		}
		detail = built
		return nil
	})
	return detail, err
}
