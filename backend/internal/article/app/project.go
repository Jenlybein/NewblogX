package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
)

// 本文件是「项目」的读者侧只读用例（#193）。
//
// 项目与合集并列，读口径与合集**逐字一致**（见 spec #189「项目」段）：
//   - 只出「展示中」的项目；
//   - 篇数只算仍公开的文章；
//   - 未展示的项目对读者按**不存在**处理，响应里不出现"未发布 / 未展示"这类内部状态；
//   - 封面为空时回退首篇公开文章的封面（回退在仓储 SQL 里一次算完）。
//
// 管理端增删改与编排属于 #197，本票不实现——所以这里没有 Create/Update/Delete。

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
