package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/hottop"
)

// 本文件是「热门置顶」运营位的用例。
//
// 与 interaction.go 里的 Top（作者置顶）**没有任何共用代码**：
// 两套数据、两条规则。重复几行编排比让一个函数用布尔参数决定
// "这次是哪种置顶"更安全——后者正是最容易写错权限的那种写法。

// ListHotTopCards 返回首页「热门置顶」区块的卡片。
//
// 公开接口，不需要登录。过滤发生在仓储 SQL 里：文章后来不再公开的置顶行
// 保留在表里（后台要能看见并清理），但不会出现在这里。
func (s *Service) ListHotTopCards(ctx context.Context) ([]hottop.Card, error) {
	var cards []hottop.Card
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, err = r.HotTops().ListCards(ctx)
		return err
	})
	return cards, err
}

// ListHotTopItems 返回后台「热门置顶」页的全部行（含已失效）。
//
// 只给管理员：它会把"文章已隐藏/已删除"这类内部状态摆出来。
func (s *Service) ListHotTopItems(ctx context.Context, viewer domain.Viewer) ([]hottop.AdminItem, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var items []hottop.AdminItem
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		items, err = r.HotTops().ListAdminItems(ctx)
		return err
	})
	return items, err
}

// SetHotTop 设置或取消热门置顶。两个方向都幂等。
//
// 设置时四条规则（ADR-0017 决策 5）：
//  1. 只有管理员；
//  2. 文章必须已发布且可见——不给"先置顶、发布后自动生效"，那会让首页出现
//     一条指向草稿的死链；
//  3. 最多 hottop.MaxItems 条，超限返回可读错误（不是数据库 1062）；
//  4. 已置顶的文章重复设置是空操作：既不报满，也不把它挪到末尾。
//
// 取消时**不查文章**：文章被删掉后置顶行仍保留（后台标「已失效」），
// 一键清理必须能删掉这种"文章已经不存在"的行。
func (s *Service) SetHotTop(ctx context.Context, cmd HotTopCmd) error {
	if !cmd.Actor.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if cmd.Remove {
			return r.HotTops().Delete(ctx, cmd.ArticleID)
		}

		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !hottop.CanBePinned(a) {
			return hottop.ErrNotPinnable
		}

		exists, err := r.HotTops().Exists(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		count, err := r.HotTops().Count(ctx)
		if err != nil {
			return err
		}
		if count >= hottop.MaxItems {
			return hottop.ErrLimitExceeded
		}

		_, err = r.HotTops().Save(ctx, cmd.ArticleID)
		return err
	})
}

// ReplaceHotTopOrder 用完整有序列表原子重排热门置顶。
//
// 输入必须是当前置顶集合的一个排列（hottop.CheckOrder）：集合不一致就整单拒绝。
// 刻意不做"缺的删掉"这种宽松语义——后台排序按钮拿到的是上一次读到的列表，
// 一个过期页面点一次「上移」就会把别人刚加的置顶静默删掉。
//
// 单条删除仍然只有 SetHotTop(Remove) 一条路，两者职责不重叠。
func (s *Service) ReplaceHotTopOrder(ctx context.Context, viewer domain.Viewer, ids []domain.ArticleID) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		current, err := r.HotTops().ListIDs(ctx)
		if err != nil {
			return err
		}
		if err := hottop.CheckOrder(current, ids); err != nil {
			return err
		}
		return r.HotTops().UpdateSortOrder(ctx, ids)
	})
}
