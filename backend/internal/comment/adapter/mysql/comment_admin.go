package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/comment/domain"
)

// 本文件是管理端评论列表的存储实现（后台「文章管理 → 评论管理」）。
//
// 与读者侧列表分开写是刻意的：读者侧只读**已发布 + 未软删**的行，管理端要
// 看**全部行**（含 status=已删除、deleted_at 非空的），WHERE 条件、作用域与
// 排序理由都不一样，混在一个方法里靠一个布尔开关切换只会让两边都难读。

// ListAdmin 分页查评论（created_at 倒序），第二个返回值表示后面还有没有。
//
// **必须 Unscoped**：gorm 的软删默认作用域会给查询自动补 `deleted_at IS NULL`，
// 而管理端恰恰要看被软删的评论（运营要核对"这条到底删没删、谁还有它"）。
// 漏掉 Unscoped 的症状不是报错，而是列表里永远看不到已删除的行 —— 一个看起来
// 完全正常的空筛选。
func (r *Repo) ListAdmin(
	ctx context.Context,
	filter domain.AdminFilter,
	page domain.AdminPage,
) ([]domain.Comment, bool, error) {
	page = page.Normalize()
	var rows []commentRow
	err := applyAdminFilters(r.db.WithContext(ctx).Unscoped(), filter).
		// 同一毫秒内写入的多条评论按 id 兜底排序：只按 created_at 排会让翻页
		// 出现"同一条在两页都出现 / 两页都漏掉"的漂移（datetime(3) 的精度不够）。
		Order("created_at DESC").Order("id DESC").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return toComments(rows), hasMore, nil
}

// applyAdminFilters 把筛选条件拼成 WHERE。
//
// 每个条件都只在"确实填了"时生效：零值 = 不筛。空关键词必须当成"没填"
// （LIKE '%%' 是能走索引的反面教材，且它看起来像一次正常查询）。
//
// 状态筛选的口径要说清：删除动作（SoftDeleteMany）写的是 deleted_at，**不改
// status**，所以 soft delete 后 status 仍然是"已发布"。因此：
//   - published = 没被软删（deleted_at IS NULL）且 status 是已发布；
//   - deleted   = 被软删（deleted_at 非空）**或** status 是已删除（存量「审核中」
//     退场时只改状态、没写 deleted_at，见 RetireExamining）；
//   - all       = 不加条件（管理端默认视图就该看到全部行）。
//
// 只看 status 会把所有软删除的评论都算成"已发布" —— 管理端的「已删除」筛选
// 会永远是空的，而「已发布」里混着一堆已经删掉的评论。
func applyAdminFilters(tx *gorm.DB, filter domain.AdminFilter) *gorm.DB {
	if filter.Keyword != "" {
		tx = tx.Where("content LIKE ?", "%"+filter.Keyword+"%")
	}
	if !filter.UserID.IsZero() {
		tx = tx.Where("user_id = ?", filter.UserID)
	}
	if filter.UserKeyword != "" {
		// 昵称读的是评论行上的快照列（user_nickname），不 JOIN 用户表：
		// 评论发出去时说什么名字就该显示什么名字，改了昵称也不该改写历史评论。
		tx = tx.Where("user_nickname LIKE ?", "%"+filter.UserKeyword+"%")
	}
	if !filter.ArticleID.IsZero() {
		tx = tx.Where("article_id = ?", filter.ArticleID)
	}
	if filter.FromAt != nil {
		tx = tx.Where("created_at >= ?", *filter.FromAt)
	}
	if filter.ToAt != nil {
		tx = tx.Where("created_at < ?", *filter.ToAt)
	}
	switch filter.Status {
	case domain.AdminStatusPublished:
		tx = tx.Where("deleted_at IS NULL AND status = ?", int8(domain.StatusPublished))
	case domain.AdminStatusDeleted:
		tx = tx.Where("deleted_at IS NOT NULL OR status = ?", int8(domain.StatusDeleted))
	}
	return tx
}
