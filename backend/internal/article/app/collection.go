package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
)

// 本文件是「合集」的用例：建合集、展示开关、读者侧读面（列表 / 详情目录树）。
//
// 与 hottop.go（热门置顶运营位）**没有共用代码**：两套数据、两套规则。
// 合集有标题/描述/封面/展示开关，且没有条数上限；热门置顶只有"哪篇文章排第几"。
// 重复几行编排比让一个函数用布尔参数决定"这次是哪种运营位"更安全。
//
// 收录与顺序自 #218 起只经目录树的整树提交（outline.go），不再有单步写用例；
// 旧扁平收录接口随旧中间表一起退役（#225）。

// CreateCollection 新建一个合集（管理员）。
//
// 四条规则：
//  1. 只有管理员——合集是管理员专属的运营产物，作者不参与创建；
//  2. 标题非空且不超长（domain 的唯一不变量）；
//  3. 新合集排在末尾：顺序号 = 当前最大 + 1；
//  4. 展示开关可以一开始就是关的——"先准备、后公布"是常见运营节奏。
func (s *Service) CreateCollection(ctx context.Context, cmd CreateCollectionCmd) (collection.ID, error) {
	if !cmd.Actor.IsAdmin() {
		return "", domain.ErrForbidden
	}

	c, err := collection.New(collection.NewParams{
		ID:          s.ids.NextCollectionID(),
		Title:       cmd.Title,
		Description: cmd.Description,
		Cover:       cmd.Cover,
		CreatorID:   cmd.Actor.UserID,
		Displayed:   cmd.Displayed,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return "", err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		order, err := r.Collections().NextSortOrder(ctx)
		if err != nil {
			return err
		}
		c.SetSortOrder(order, s.clock.Now())
		return r.Collections().Create(ctx, c)
	})
	if err != nil {
		return "", err
	}
	return c.ID(), nil
}

// UpdateCollection 改合集的标题 / 描述 / 封面（管理员）。
//
// 三条规则：
//  1. 只有管理员；
//  2. 标题沿用创建时的不变量（非空、不超长）；
//  3. 展示开关与顺序**不在这里改**——它们各自有独立的动作，一次标题编辑不该
//     顺手覆盖掉别人刚切换的运营状态。
func (s *Service) UpdateCollection(ctx context.Context, cmd UpdateCollectionCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		c, err := r.Collections().Find(ctx, cmd.CollectionID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := c.Retitle(cmd.Title, now); err != nil {
			return err
		}
		c.SetDescription(cmd.Description, now)
		c.SetCover(cmd.Cover, now)
		return r.Collections().Update(ctx, c)
	})
}

// DeleteCollection 软删一个合集（管理员）。
//
// 删合集**不删文章**：中间表的行留着（文章本身与本合集无关），读者侧因为
// 合集已经查不到而整块消失；后台列表也不再出现（软删行被仓储默认排除）。
func (s *Service) DeleteCollection(ctx context.Context, cmd DeleteCollectionCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		c, err := r.Collections().Find(ctx, cmd.CollectionID)
		if err != nil {
			return err
		}
		c.SoftDelete(s.clock.Now())
		return r.Collections().Update(ctx, c)
	})
}

// SetCollectionDisplay 打开/关闭合集的"是否展示给读者"（管理员）。
//
// 关掉后读者侧列表与详情都查不到（详情按 404 处理），后台仍可见——
// 这就是"先准备、后公布"的实现方式。
func (s *Service) SetCollectionDisplay(ctx context.Context, cmd SetCollectionDisplayCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		c, err := r.Collections().Find(ctx, cmd.CollectionID)
		if err != nil {
			return err
		}
		c.SetDisplay(cmd.Displayed, s.clock.Now())
		return r.Collections().Update(ctx, c)
	})
}

// ListCollectionCards 返回读者站合集列表的卡片（公开读，分页）。
//
// 只返回**展示中**的合集；未展示的合集留在表里（后台要能继续编辑），
// 这里查不到（过滤在仓储 SQL 里，不靠前端判断）。
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面，由 adapter 每请求
// 从站点运行时配置现读后传入：开关关闭时，一篇公开文章都凑不出的合集
// 同样不出现（合集是读者端公开读面，ADR-0015 决策 4）。
func (s *Service) ListCollectionCards(
	ctx context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]collection.Card, int64, error) {
	var (
		cards []collection.Card
		total int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, total, err = r.Collections().ListCards(ctx, onlyAdminAuthors, p.Normalize())
		return err
	})
	return cards, total, err
}

// CollectionDetail 返回合集详情（嵌套目录树），读者侧公开读。
//
// 未展示的合集对读者按**不存在**处理（不返回"这个合集未发布"——那能用来探测
// 还没公布的专题）；管理员仍能打开，后台编辑要用。
//
// 读者树只从目录表组装：失效文章节点（文章被删/变私有）在这里过滤掉，
// 空文件夹保留；封面为空时回退到深度优先第一篇公开文章的封面。
func (s *Service) CollectionDetail(
	ctx context.Context, id collection.ID, viewer domain.Viewer, onlyAdminAuthors bool,
) (*collection.Detail, error) {
	var detail *collection.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		c, err := repos.Collections().Find(ctx, id)
		if err != nil {
			return err
		}
		if !c.Displayed() && !viewer.IsAdmin() {
			return collection.ErrNotFound
		}
		// 合集详情只读自己的具名树表，与后台编辑器、阅读页左栏、列表卡片
		// 同一份数据；项目侧由 ProjectDetail 对称处理。
		r, err := namedRepos(ctx, repos, outline.OwnerCollection)
		if err != nil {
			return err
		}
		ownerKind, ownerID := outline.OwnerCollection, outline.ID(id)
		tree, err := s.readerOutline(ctx, r, ownerKind, ownerID, onlyAdminAuthors, c.Cover())
		if err != nil {
			return err
		}
		detail = &collection.Detail{
			Collection:   c,
			Tree:         tree.Tree,
			ArticleCount: tree.ArticleCount,
			Cover:        tree.Cover,
		}
		return nil
	})
	return detail, err
}

// ListAdminCollectionCards 返回后台合集列表（含未展示的合集）。
//
// 只给管理员：它会摆出"这个合集还没展示"这类运营状态。
func (s *Service) ListAdminCollectionCards(ctx context.Context, viewer domain.Viewer) ([]collection.AdminCard, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var cards []collection.AdminCard
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, err = r.Collections().ListAdminCards(ctx)
		return err
	})
	return cards, err
}
