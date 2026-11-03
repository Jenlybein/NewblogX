package app

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
)

// 本文件是「项目」的用例：读者侧只读（#193）+ 管理端增删改与编排（#197）。
//
// 项目与合集并列，读口径与写语义与合集**逐字一致**（见 spec #189「项目」段）：
//   - 只出「展示中」的项目；
//   - 篇数只算仍公开的文章；
//   - 未展示的项目对读者按**不存在**处理，响应里不出现"未发布 / 未展示"这类内部状态；
//   - 封面为空时回退首篇公开文章的封面（回退在仓储 SQL 里一次算完）；
//   - 写上全部挂管理员，与非管理员共用既有鉴权错误码（domain.ErrForbidden）；
//   - 关联行沿用合集的失效口径：文章不再公开时行保留、读者侧联表过滤、后台可清理。
//
// 两套集合不共用代码：项目有自己的表、自己的 ID 类型、自己的后台页；重复
// 几行编排比让一个函数用布尔参数决定"这次是哪种运营位"更安全。

// ListProjectCards 返回读者站项目列表的卡片（公开读，分页）。
//
// 只返回**展示中**的项目；未展示的项目留在表里（后台 #197 要能继续编辑），
// 这里查不到（过滤在仓储 SQL 里，不靠前端判断）。
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面，由 adapter 每请求
// 从站点运行时配置现读后传入：开关关闭时，一篇公开文章都凑不出的项目
// 同样不出现（项目是读者端公开读面，ADR-0017 决策 4）。
func (s *Service) ListProjectCards(
	ctx context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]project.Card, int64, error) {
	var (
		cards []project.Card
		total int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, total, err = r.Projects().ListCards(ctx, onlyAdminAuthors, p.Normalize())
		return err
	})
	return cards, total, err
}

// ProjectDetail 返回项目详情（含按顺序排列的文章），读者侧公开读。
//
// 未展示的项目对读者按**不存在**处理（不返回"这个项目未发布"——那能用来探测
// 还没公布的编排）；管理员读面属于 #197，会走另一条入口。
//
// 收录里的失效行（文章被删/变私有）由仓储 JOIN 过滤掉，不靠前端判断。
func (s *Service) ProjectDetail(
	ctx context.Context, id project.ID, onlyAdminAuthors bool,
) (*project.Detail, error) {
	var detail *project.Detail
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p, err := r.Projects().Find(ctx, id)
		if err != nil {
			return err
		}
		if !p.Displayed() {
			return project.ErrNotFound
		}
		articles, err := r.Projects().ListIncludedArticles(ctx, id, onlyAdminAuthors)
		if err != nil {
			return err
		}
		detail = &project.Detail{Project: p, Articles: articles}
		return nil
	})
	return detail, err
}

// CreateProject 新建一个项目（管理员）。
//
// 四条规则（与 CreateCollection 逐条一致）：
//  1. 只有管理员——项目是管理员专属的运营产物，作者不参与创建；
//  2. 标题非空且不超长（domain 的唯一不变量）；
//  3. 新项目排在末尾：顺序号 = 当前最大 + 1；
//  4. 展示开关可以一开始就是关的——"先准备、后公布"是常见运营节奏。
func (s *Service) CreateProject(ctx context.Context, cmd CreateProjectCmd) (project.ID, error) {
	if !cmd.Actor.IsAdmin() {
		return "", domain.ErrForbidden
	}

	p, err := project.New(project.NewParams{
		ID:          s.ids.NextProjectID(),
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
		order, err := r.Projects().NextSortOrder(ctx)
		if err != nil {
			return err
		}
		p.SetSortOrder(order, s.clock.Now())
		return r.Projects().Create(ctx, p)
	})
	if err != nil {
		return "", err
	}
	return p.ID(), nil
}

// UpdateProject 改项目的标题 / 描述 / 封面（管理员）。
//
// 三条规则：
//  1. 只有管理员；
//  2. 标题沿用创建时的不变量（非空、不超长）；
//  3. 展示开关与顺序**不在这里改**——它们各自有独立的动作，一次标题编辑不该
//     顺手覆盖掉别人刚切换的运营状态。
func (s *Service) UpdateProject(ctx context.Context, cmd UpdateProjectCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p, err := r.Projects().Find(ctx, cmd.ProjectID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := p.Retitle(cmd.Title, now); err != nil {
			return err
		}
		p.SetDescription(cmd.Description, now)
		p.SetCover(cmd.Cover, now)
		return r.Projects().Update(ctx, p)
	})
}

// DeleteProject 软删一个项目（管理员）。
//
// 删项目**不删文章**：中间表的行留着（文章本身与本项目无关），读者侧因为
// 项目已经查不到而整块消失；后台列表也不再出现（软删行被仓储默认排除）。
func (s *Service) DeleteProject(ctx context.Context, cmd DeleteProjectCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p, err := r.Projects().Find(ctx, cmd.ProjectID)
		if err != nil {
			return err
		}
		p.SoftDelete(s.clock.Now())
		return r.Projects().Update(ctx, p)
	})
}

// IncludeProjectArticle 把一篇文章收录进项目（管理员）。
//
// 三条规则：
//  1. 只有管理员；
//  2. 文章必须已发布且可见——不给"先收录、发布后自动生效"，那会让项目里出现
//     一条点进去 404 的条目；
//  3. 同一项目内不重复收录（可读错误，不是数据库 1062）。
//
// 换序不在这个动作里：收录永远排在末尾，排序走 ReplaceProjectArticleOrder。
func (s *Service) IncludeProjectArticle(ctx context.Context, cmd IncludeProjectArticleCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Projects().Find(ctx, cmd.ProjectID); err != nil {
			return err
		}

		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !project.CanBeIncluded(a) {
			return project.ErrNotIncludable
		}

		created, err := r.Projects().AppendItem(ctx, cmd.ProjectID, project.ArticleID(cmd.ArticleID))
		if err != nil {
			return err
		}
		if !created {
			return project.ErrDuplicateArticle
		}
		return nil
	})
}

// IncludeProjectArticles 批量把多篇文章收录进项目（管理员）。
//
// 与单篇收录的区别只有一处：结果**逐篇分类**而不是整单成败。
// 全部发生在同一个事务里：中途出现基础设施错误时整单回滚，不会出现半截状态。
// 不存在的文章记为 Missing 而不是中断整单——运营勾选列表里的过期项
// 不该阻塞其它条目。
func (s *Service) IncludeProjectArticles(
	ctx context.Context, cmd IncludeProjectArticlesCmd,
) ([]project.BatchIncludeResult, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	results := make([]project.BatchIncludeResult, 0, len(cmd.ArticleIDs))
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Projects().Find(ctx, cmd.ProjectID); err != nil {
			return err
		}
		for _, articleID := range cmd.ArticleIDs {
			outcome, err := s.includeProjectOne(ctx, r, cmd.ProjectID, articleID)
			if err != nil {
				return err
			}
			results = append(results, project.BatchIncludeResult{
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

// includeProjectOne 是批量收录里的一篇；只返回结局，不把"不可收录"升级成错误。
func (s *Service) includeProjectOne(
	ctx context.Context, r Repos, id project.ID, articleID domain.ArticleID,
) (project.IncludeOutcome, error) {
	a, err := r.Articles().Find(ctx, articleID)
	if err != nil {
		if errors.Is(err, domain.ErrArticleNotFound) {
			return project.IncludeOutcomeMissing, nil
		}
		return "", err
	}
	if !project.CanBeIncluded(a) {
		return project.IncludeOutcomeNotIncludable, nil
	}

	created, err := r.Projects().AppendItem(ctx, id, project.ArticleID(articleID))
	if err != nil {
		return "", err
	}
	if !created {
		return project.IncludeOutcomeDuplicated, nil
	}
	return project.IncludeOutcomeIncluded, nil
}

// RemoveProjectArticle 从项目中移除一篇文章（管理员）。
//
// 幂等：不在项目里也返回成功（后台批量清理时不该因为一行过期数据整单失败）。
// 移除**不删文章**：中间行软删，剩余序号压实成 1..n。
func (s *Service) RemoveProjectArticle(ctx context.Context, cmd RemoveProjectArticleCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Projects().Find(ctx, cmd.ProjectID); err != nil {
			return err
		}
		return r.Projects().RemoveItem(ctx, cmd.ProjectID, cmd.ArticleID)
	})
}

// SetProjectDisplay 打开/关闭项目的"是否展示给读者"（管理员）。
//
// 关掉后读者侧列表与详情都查不到（详情按 404 处理），后台仍可见——
// 这就是"先准备、后公布"的实现方式。
func (s *Service) SetProjectDisplay(ctx context.Context, cmd SetProjectDisplayCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p, err := r.Projects().Find(ctx, cmd.ProjectID)
		if err != nil {
			return err
		}
		p.SetDisplay(cmd.Displayed, s.clock.Now())
		return r.Projects().Update(ctx, p)
	})
}

// ReplaceProjectArticleOrder 用完整有序列表原子重排项目内的文章（管理员）。
//
// 输入必须是当前收录集合的一个排列（project.CheckOrder）：集合不一致就整单
// 拒绝。刻意不做"缺的删掉"这种宽松语义——后台排序按钮拿到的是上一次读到的
// 列表，一个过期页面点一次「上移」就会把别人刚收录的那篇静默删掉。
func (s *Service) ReplaceProjectArticleOrder(
	ctx context.Context, viewer domain.Viewer, id project.ID, articleIDs []project.ArticleID,
) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if _, err := r.Projects().Find(ctx, id); err != nil {
			return err
		}
		current, err := r.Projects().ListItemIDs(ctx, id)
		if err != nil {
			return err
		}
		if err := project.CheckOrder(current, articleIDs); err != nil {
			return err
		}
		return r.Projects().UpdateItemOrder(ctx, id, articleIDs)
	})
}

// ReplaceProjectSortOrder 用完整有序列表原子重排项目之间的展示顺序（管理员）。
//
// 输入必须是当前全部项目的排列（project.CheckSortOrder）：集合不一致整单拒绝。
// 与项目内排序同一条语义，只是对象换成了项目自身；落库的是
// project_models.sort_order，读者列表页与后台列表都按它排序。
func (s *Service) ReplaceProjectSortOrder(
	ctx context.Context, viewer domain.Viewer, ids []project.ID,
) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		current, err := r.Projects().ListIDs(ctx)
		if err != nil {
			return err
		}
		if err := project.CheckSortOrder(current, ids); err != nil {
			return err
		}
		return r.Projects().UpdateSortOrders(ctx, ids)
	})
}

// ListAdminProjectCards 返回后台项目列表（含未展示的项目）。
//
// 只给管理员：它会摆出"这个项目还没展示"这类运营状态。
func (s *Service) ListAdminProjectCards(ctx context.Context, viewer domain.Viewer) ([]project.AdminCard, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var cards []project.AdminCard
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, err = r.Projects().ListAdminCards(ctx)
		return err
	})
	return cards, err
}

// ListProjectAdminItems 返回后台「项目编排」页的全部行（含已失效占位）。
//
// 只给管理员：它会把"文章已隐藏/退回草稿/已删除"这类内部状态摆出来。
// 失效判定不在 app 里另写——AdminItem.Stale() 直接用三个运营位共用的
// domain.ArticleState.IsStale。
func (s *Service) ListProjectAdminItems(
	ctx context.Context, viewer domain.Viewer, id project.ID,
) ([]project.AdminItem, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var items []project.AdminItem
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		// 先确认项目存在（软删/不存在都按不存在处理），后台不能把失效行
		// 挂在一个已经不存在的项目下显示。
		if _, err := r.Projects().Find(ctx, id); err != nil {
			return err
		}
		var err error
		items, err = r.Projects().ListAdminItems(ctx, id)
		return err
	})
	return items, err
}

// CleanupStaleProjectArticles 一键清理项目里所有失效的收录行（管理员）。
//
// 三条规则（与合集逐条一致）：
//  1. 只有管理员；
//  2. 只删**失效**行：仍公开的收录不动，哪怕它排在失效行中间；
//  3. 幂等：没有失效行时返回 0 且不报错，重复点击不会删到生效行。
//
// 失效集合由仓储一次列出后，用与后台标记同一个 AdminItem.Stale() 判定，
// 再复用单条移除（软删 + 压实序号）。删除**不删文章**，只是解除收录关系。
func (s *Service) CleanupStaleProjectArticles(
	ctx context.Context, viewer domain.Viewer, id project.ID,
) (int, error) {
	if !viewer.IsAdmin() {
		return 0, domain.ErrForbidden
	}

	removed := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		removed = 0
		if _, err := r.Projects().Find(ctx, id); err != nil {
			return err
		}
		items, err := r.Projects().ListAdminItems(ctx, id)
		if err != nil {
			return err
		}
		for _, item := range items {
			if !item.Stale() {
				continue
			}
			if err := r.Projects().RemoveItem(ctx, id, item.ArticleID); err != nil {
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
