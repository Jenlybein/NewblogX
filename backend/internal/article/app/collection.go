package app

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
)

// 本文件是「合集」的用例：建合集、收录一篇、移除收录、排序、读者侧读面。
//
// 与 hottop.go（热门置顶运营位）**没有共用代码**：两套数据、两套规则。
// 合集有标题/描述/封面/展示开关，且没有条数上限；热门置顶只有"哪篇文章排第几"。
// 重复几行编排比让一个函数用布尔参数决定"这次是哪种运营位"更安全。

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

// IncludeArticle 把一篇文章收录进合集（管理员）。
//
// 三条规则：
//  1. 只有管理员；
//  2. 文章必须已发布且可见——不给"先收录、发布后自动生效"，那会让合集里出现
//     一条点进去 404 的条目；
//  3. 同一合集内不重复收录（可读错误，不是数据库 1062）。
//
// 移动/换序不在这个动作里：收录永远排在末尾，排序走 ReplaceCollectionArticleOrder。
func (s *Service) IncludeArticle(ctx context.Context, cmd IncludeArticleCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Collections().Find(ctx, cmd.CollectionID); err != nil {
			return err
		}

		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !collection.CanBeIncluded(a) {
			return collection.ErrNotIncludable
		}

		created, err := r.Collections().AppendItem(ctx, cmd.CollectionID, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !created {
			return collection.ErrDuplicateArticle
		}
		return nil
	})
}

// IncludeArticles 批量把多篇文章收录进合集（管理员）。
//
// 与 IncludeArticle 的区别只有一处：结果**逐篇分类**而不是整单成败。
// 三条规则同样成立：
//  1. 只有管理员（否则整单 403，一篇都不落库）；
//  2. 每篇文章都必须已发布且可见，不可收录的记为 NotIncludable；
//  3. 已收录的记为 Duplicated，不新增、不改序号（不会把旧条目挪到末尾）。
//
// 全部发生在同一个事务里：中途出现基础设施错误时整单回滚，
// 不会出现"前几篇进去了、后几篇没进去"的半截状态。
// 不存在的文章记为 Missing 而不是中断整单——运营勾选列表里的过期项
// 不该阻塞其它条目。
func (s *Service) IncludeArticles(
	ctx context.Context, cmd IncludeArticlesCmd,
) ([]collection.BatchIncludeResult, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	results := make([]collection.BatchIncludeResult, 0, len(cmd.ArticleIDs))
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Collections().Find(ctx, cmd.CollectionID); err != nil {
			return err
		}
		for _, articleID := range cmd.ArticleIDs {
			outcome, err := s.includeOne(ctx, r, cmd.CollectionID, articleID)
			if err != nil {
				return err
			}
			results = append(results, collection.BatchIncludeResult{
				ArticleID: articleID,
				Outcome:   outcome,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// includeOne 是批量收录里的一篇；只返回结局，不把"不可收录"升级成错误。
func (s *Service) includeOne(
	ctx context.Context, r Repos, id collection.ID, articleID domain.ArticleID,
) (collection.IncludeOutcome, error) {
	a, err := r.Articles().Find(ctx, articleID)
	if err != nil {
		if errors.Is(err, domain.ErrArticleNotFound) {
			return collection.IncludeOutcomeMissing, nil
		}
		return "", err
	}
	if !collection.CanBeIncluded(a) {
		return collection.IncludeOutcomeNotIncludable, nil
	}

	created, err := r.Collections().AppendItem(ctx, id, collection.ArticleID(articleID))
	if err != nil {
		return "", err
	}
	if !created {
		return collection.IncludeOutcomeDuplicated, nil
	}
	return collection.IncludeOutcomeIncluded, nil
}

// RemoveArticle 从合集中移除一篇文章（管理员）。
//
// 幂等：不在合集里也返回成功（后台批量清理时不该因为一行过期数据整单失败）。
// 移除**不删文章**：中间行软删，剩余序号压实成 1..n。
func (s *Service) RemoveArticle(ctx context.Context, cmd RemoveArticleCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Collections().Find(ctx, cmd.CollectionID); err != nil {
			return err
		}
		return r.Collections().RemoveItem(ctx, cmd.CollectionID, cmd.ArticleID)
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

// ReplaceCollectionArticleOrder 用完整有序列表原子重排合集内的文章（管理员）。
//
// 输入必须是当前收录集合的一个排列（collection.CheckOrder）：集合不一致就整单
// 拒绝。刻意不做"缺的删掉"这种宽松语义——后台排序按钮拿到的是上一次读到的
// 列表，一个过期页面点一次「上移」就会把别人刚收录的那篇静默删掉。
func (s *Service) ReplaceCollectionArticleOrder(
	ctx context.Context, viewer domain.Viewer, id collection.ID, articleIDs []collection.ArticleID,
) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Collections().Find(ctx, id); err != nil {
			return err
		}
		current, err := r.Collections().ListItemIDs(ctx, id)
		if err != nil {
			return err
		}
		if err := collection.CheckOrder(current, articleIDs); err != nil {
			return err
		}
		return r.Collections().UpdateItemOrder(ctx, id, articleIDs)
	})
}

// ListCollectionCards 返回读者站合集列表的卡片（公开读，分页）。
//
// 只返回**展示中**的合集；未展示的合集留在表里（后台要能继续编辑），
// 这里查不到（过滤在仓储 SQL 里，不靠前端判断）。
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面，由 adapter 每请求
// 从站点运行时配置现读后传入：开关关闭时，一篇公开文章都凑不出的合集
// 同样不出现（合集是读者端公开读面，ADR-0017 决策 4）。
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
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		c, err := r.Collections().Find(ctx, id)
		if err != nil {
			return err
		}
		if !c.Displayed() && !viewer.IsAdmin() {
			return collection.ErrNotFound
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

// ReplaceCollectionSortOrder 用完整有序列表原子重排合集之间的展示顺序（管理员）。
//
// 输入必须是当前全部合集的排列（collection.CheckSortOrder）：集合不一致整单拒绝。
// 与合集内排序同一条语义，只是对象换成了合集自身；落库的是
// collection_models.sort_order，读者列表页与后台列表都按它排序。
func (s *Service) ReplaceCollectionSortOrder(
	ctx context.Context, viewer domain.Viewer, ids []collection.ID,
) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		current, err := r.Collections().ListIDs(ctx)
		if err != nil {
			return err
		}
		if err := collection.CheckSortOrder(current, ids); err != nil {
			return err
		}
		return r.Collections().UpdateSortOrders(ctx, ids)
	})
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

// ListCollectionAdminItems 返回后台「合集收录」页的全部行（含已失效占位）。
//
// 只给管理员：它会把"文章已隐藏/退回草稿/已删除"这类内部状态摆出来。
// 失效判定不在 app 里另写——AdminItem.Stale() 直接用两个运营位共用的
// domain.ArticleState.IsStale，口径与热门置顶逐字一致。
func (s *Service) ListCollectionAdminItems(
	ctx context.Context, viewer domain.Viewer, id collection.ID,
) ([]collection.AdminItem, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var items []collection.AdminItem
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		// 先确认合集存在（软删/不存在都按不存在处理），后台不能把失效行
		// 挂在一个已经不存在的合集下显示。
		if _, err := r.Collections().Find(ctx, id); err != nil {
			return err
		}
		var err error
		items, err = r.Collections().ListAdminItems(ctx, id)
		return err
	})
	return items, err
}

// CleanupStaleCollectionArticles 一键清理合集里所有失效的收录行（管理员）。
//
// 三条规则：
//  1. 只有管理员；
//  2. 只删**失效**行：仍公开的收录不动，哪怕它排在失效行中间；
//  3. 幂等：没有失效行时返回 0 且不报错，重复点击不会删到生效行。
//
// 失效集合由仓储一次列出后，用与后台标记同一个 AdminItem.Stale() 判定，
// 再复用单条移除（软删 + 压实序号）。删除**不删文章**，只是解除收录关系。
func (s *Service) CleanupStaleCollectionArticles(
	ctx context.Context, viewer domain.Viewer, id collection.ID,
) (int, error) {
	if !viewer.IsAdmin() {
		return 0, domain.ErrForbidden
	}

	removed := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		removed = 0
		if _, err := r.Collections().Find(ctx, id); err != nil {
			return err
		}
		items, err := r.Collections().ListAdminItems(ctx, id)
		if err != nil {
			return err
		}
		for _, item := range items {
			if !item.Stale() {
				continue
			}
			if err := r.Collections().RemoveItem(ctx, id, item.ArticleID); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
