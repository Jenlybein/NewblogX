package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
)

// 本文件是「项目」的用例：读者侧只读（#193）+ 管理端增删改、展示开关与
// 项目之间排序（#197）。
//
// 项目与合集并列，读口径与写语义与合集**逐字一致**（见 spec #189「项目」段）：
//   - 只出「展示中」的项目；
//   - 篇数只算仍公开的文章；
//   - 未展示的项目对读者按**不存在**处理，响应里不出现"未发布 / 未展示"这类内部状态；
//   - 封面为空时回退首篇公开文章的封面（回退在仓储 SQL 里一次算完）；
//   - 写上全部挂管理员，与非管理员共用既有鉴权错误码（domain.ErrForbidden）。
//
// 两套集合不共用代码：项目有自己的表、自己的 ID 类型、自己的后台页；重复
// 几行编排比让一个函数用布尔参数决定"这次是哪种运营位"更安全。
//
// 收录与顺序只经目录树的整树提交（outline.go）；旧扁平收录接口随旧中间表
// 一起退役（#225）。

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

// ProjectDetail 返回项目详情（嵌套目录树），读者侧公开读。
//
// 未展示的项目对读者按**不存在**处理（不返回"这个项目未发布"——那能用来探测
// 还没公布的编排）；管理员读面属于 #197，会走另一条入口。
//
// 与合集共用同一份读者树组装：只出公开文章节点、空文件夹保留、封面为空时
// 回退到深度优先第一篇公开文章。
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
		ownerKind, ownerID := outline.OwnerProject, outline.ID(id)
		tree, err := s.readerOutline(ctx, r, ownerKind, ownerID, onlyAdminAuthors, p.Cover())
		if err != nil {
			return err
		}
		detail = &project.Detail{
			Project:      p,
			Tree:         tree.Tree,
			ArticleCount: tree.ArticleCount,
			Cover:        tree.Cover,
		}
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
