package app

import (
	"context"
	"time"

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
		return nil, grouping.ErrWiring
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
		return repo.DeleteGroup(ctx, cmd.ID, s.clock.Now())
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

		// 实体全集先取出来：既用来补"没有归属行的实体"，也用来滤掉"归属行还在、
		// 实体已软删"的幽灵 ID。
		//
		// 幽灵 ID 不是理论问题：软删一个合集不会连带删它的归属行（删实体走的是
		// 实体自己的用例），不滤的话后台读面会把一个已经删掉的 ID 放进 groups[].items，
		// 前端点进去是 404。
		all, err := allOwnerIDs(ctx, repos, kind)
		if err != nil {
			return err
		}
		exists := make(map[ctype.ID]struct{}, len(all))
		for _, owner := range all {
			exists[owner] = struct{}{}
		}

		byGroup := make(map[grouping.ID][]ctype.ID, len(groups))
		placed := make(map[ctype.ID]struct{}, len(assignments))
		ungrouped := make([]ctype.ID, 0, len(assignments))
		for _, a := range assignments {
			if _, ok := exists[a.OwnerID]; !ok {
				continue
			}
			placed[a.OwnerID] = struct{}{}
			if a.GroupID.IsZero() {
				ungrouped = append(ungrouped, a.OwnerID)
				continue
			}
			byGroup[a.GroupID] = append(byGroup[a.GroupID], a.OwnerID)
		}

		// 从来没有归属行的实体也算未分组（接在已有未分组条目之后）。
		//
		// 只遍历归属行会让"新建但还没拖过"的合集在后台读面里消失 ——
		// 前端会把它补回来，但读模型本身不该依赖调用方兜底。
		for _, owner := range all {
			if _, ok := placed[owner]; ok {
				continue
			}
			ungrouped = append(ungrouped, owner)
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

// allOwnerIDs 取该实体下**全部**未删实体的 ID。
//
// 与 visibleOwnerIDs（读者侧）的区别：这里不按展示开关过滤 —— 后台编排要能
// 拖到隐藏的实体上，否则"关掉展示的那个合集"永远归不了组。
func allOwnerIDs(ctx context.Context, repos Repos, kind outline.OwnerKind) ([]ctype.ID, error) {
	switch kind {
	case outline.OwnerCollection:
		cards, err := repos.Collections().ListAdminCards(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ctype.ID, 0, len(cards))
		for _, card := range cards {
			out = append(out, ctype.ID(card.ID))
		}
		return out, nil
	case outline.OwnerProject:
		cards, err := repos.Projects().ListAdminCards(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ctype.ID, 0, len(cards))
		for _, card := range cards {
			out = append(out, ctype.ID(card.ID))
		}
		return out, nil
	default:
		return nil, grouping.ErrOrderMismatch
	}
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

		groupIDs := make([]grouping.ID, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID())
		}
		// 全集必须取**实体**全集，不是"已有归属行的条目"。
		//
		// 从来没有被拖进过分组的实体没有归属行；只按归属行算全集的话，
		// 后台第一次拖动就会因为"提交里多了一个库不知道的条目"被整单拒绝 ——
		// 也就是这个功能在干净数据上根本用不了。
		owners, err := allOwnerIDs(ctx, repos, cmd.OwnerKind)
		if err != nil {
			return err
		}
		if err := grouping.CheckOrder(groupIDs, owners, cmd.Tree); err != nil {
			return err
		}
		return repo.ReplaceOrder(ctx, cmd.Tree, s.clock.Now())
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

// composeFields 是"字段 + 整棵树"提交里与实体无关的那一半。
//
// 两个实体的字段（标题 / 描述 / 封面）与提交顺序完全一致，差异只有
// "用哪个仓储、拿哪个 ID 当树归属"。把这半段抽出来，两个 Compose 各自只剩
// 一次 Find + 一次 Update —— 之前两份 30 行只差三行，改一处必然漏另一处。
type composeFields struct {
	OwnerKind   outline.OwnerKind
	OwnerID     outline.ID
	Title       string
	Description string
	Cover       string
	Nodes       []OutlineInputNode
	// find 读回实体、retitle/setDescription/setCover 改它、update 落库。
	//
	// find / update 收 repos：实体仓储必须来自**当前事务**，不能闭包住外层句柄。
	find           func(ctx context.Context, repos Repos) error
	retitle        func(now time.Time) error
	setDescription func(now time.Time)
	setCover       func(now time.Time)
	update         func(ctx context.Context, repos Repos) error
}

// apply 在一个事务里先写字段、再整树提交，返回提交后的后台树读面。
func (s *Service) compose(ctx context.Context, actor domain.Viewer, in composeFields) (*outline.Detail, error) {
	if !actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	now := s.clock.Now()
	var detail *outline.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		if err := in.find(ctx, repos); err != nil {
			return err
		}
		if err := in.retitle(now); err != nil {
			return err
		}
		in.setDescription(now)
		in.setCover(now)
		if err := in.update(ctx, repos); err != nil {
			return err
		}

		built, err := s.saveNamedOutlineInTx(ctx, repos, SaveNamedOutlineCmd{
			Actor:     actor,
			OwnerKind: in.OwnerKind,
			OwnerID:   in.OwnerID,
			Nodes:     in.Nodes,
		}, now)
		if err != nil {
			return err
		}
		detail = built
		return nil
	})
	return detail, err
}

// ComposeCollection 在一个事务里写合集字段与整棵目录树（管理员）。
//
// 为什么合成一个端点：编辑弹窗里"改标题 + 拖完树"是一次保存动作。分成两个
// 请求，任何一个失败都会留下"标题改了但树没改"的半截状态，而用户只点了一次
// 保存 —— 他没有任何线索知道该重试哪一个。
func (s *Service) ComposeCollection(ctx context.Context, cmd ComposeCollectionCmd) (*outline.Detail, error) {
	var entity *collection.Collection
	return s.compose(ctx, cmd.Actor, composeFields{
		OwnerKind:   outline.OwnerCollection,
		OwnerID:     outline.ID(cmd.ID),
		Title:       cmd.Title,
		Description: cmd.Description,
		Cover:       cmd.Cover,
		Nodes:       cmd.Nodes,
		find: func(ctx context.Context, repos Repos) (err error) {
			entity, err = repos.Collections().Find(ctx, cmd.ID)
			return err
		},
		retitle:        func(now time.Time) error { return entity.Retitle(cmd.Title, now) },
		setDescription: func(now time.Time) { entity.SetDescription(cmd.Description, now) },
		setCover:       func(now time.Time) { entity.SetCover(cmd.Cover, now) },
		update:         func(ctx context.Context, repos Repos) error { return repos.Collections().Update(ctx, entity) },
	})
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
	var entity *project.Project
	return s.compose(ctx, cmd.Actor, composeFields{
		OwnerKind:   outline.OwnerProject,
		OwnerID:     outline.ID(cmd.ID),
		Title:       cmd.Title,
		Description: cmd.Description,
		Cover:       cmd.Cover,
		Nodes:       cmd.Nodes,
		find: func(ctx context.Context, repos Repos) (err error) {
			entity, err = repos.Projects().Find(ctx, cmd.ID)
			return err
		},
		retitle:        func(now time.Time) error { return entity.Retitle(cmd.Title, now) },
		setDescription: func(now time.Time) { entity.SetDescription(cmd.Description, now) },
		setCover:       func(now time.Time) { entity.SetCover(cmd.Cover, now) },
		update:         func(ctx context.Context, repos Repos) error { return repos.Projects().Update(ctx, entity) },
	})
}

// PublicGroupTree 是读者侧的分组读面（票 353）。
//
// 与后台读面的区别只有两条：
//  1. 只列**展示中**的实体（关掉展示的不该出现在任何分组里）；
//  2. **没有未分组那一段** —— 读者侧不渲染「未分组」标题，那些实体作为
//     所有分组之后的尾块直接平铺（"未分组自动隐藏"是后台才有的行为）。
//
// 分组标题在读者侧只是视觉分段，不承担权限或过滤语义：一个实体是否可见
// 仍由列表端点自己的规则决定，这里只负责"谁和谁在一起、谁在前"。
func (s *Service) PublicGroupTree(ctx context.Context, kind outline.OwnerKind) (*GroupTree, error) {
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

		visible, err := visibleOwnerIDs(ctx, repos, kind)
		if err != nil {
			return err
		}

		byGroup := make(map[grouping.ID][]ctype.ID, len(groups))
		trailing := make([]ctype.ID, 0, len(assignments))
		for _, a := range assignments {
			if _, ok := visible[a.OwnerID]; !ok {
				continue
			}
			if a.GroupID.IsZero() {
				trailing = append(trailing, a.OwnerID)
				continue
			}
			byGroup[a.GroupID] = append(byGroup[a.GroupID], a.OwnerID)
		}

		out := &GroupTree{Groups: make([]GroupNode, 0, len(groups)), Ungrouped: trailing}
		for _, g := range groups {
			items := byGroup[g.ID()]
			// 空分组在读者侧不渲染：一个只有标题、下面什么都没有的分段
			// 只会让页面看起来坏了。
			if len(items) == 0 {
				continue
			}
			out.Groups = append(out.Groups, GroupNode{ID: g.ID(), Title: g.Title(), Items: items})
		}
		tree = out
		return nil
	})
	return tree, err
}

// visibleOwnerIDs 取该实体下**展示中**的全部 ID。
//
// 用列表仓储而不是直接查表：可见性规则（展示开关、非管理员作者过滤）只写在
// 仓储 SQL 里一份，这里再拼一条 SQL 就是第二份真相。
func visibleOwnerIDs(ctx context.Context, repos Repos, kind outline.OwnerKind) (map[ctype.ID]struct{}, error) {
	out := make(map[ctype.ID]struct{})
	switch kind {
	case outline.OwnerCollection:
		cards, _, err := repos.Collections().ListCards(ctx, false, domain.Page{Limit: maxPublicCards})
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			out[ctype.ID(card.ID)] = struct{}{}
		}
	case outline.OwnerProject:
		cards, _, err := repos.Projects().ListCards(ctx, false, domain.Page{Limit: maxPublicCards})
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			out[ctype.ID(card.ID)] = struct{}{}
		}
	default:
		return nil, grouping.ErrOrderMismatch
	}
	return out, nil
}

// maxPublicCards 是公开读面一次最多取多少个实体。
//
// 分组读面只用来决定"谁和谁在一起、谁在前"，不负责分页：列表本身另有分页端点。
// 上限存在的意义是防止一次读出几万行；超过这个量的站点早该换成分页读面。
const maxPublicCards = 500
