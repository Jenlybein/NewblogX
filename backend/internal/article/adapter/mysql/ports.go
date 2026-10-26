package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// authorStats 是 app.AuthorStatsPort 的实现。
//
// 这是唯一的跨模块**写**。模式是"本包出事务句柄，对方出写入逻辑"：
// 同步、同事务，改成异步会让"删完文章作者文章数还是旧的"成为用户可见的行为变化。
//
// 具体 SQL 不在本包：写 user_stat 是 user 模块的知识。apply 由装配层注入，
// 本包只把当前事务句柄递过去，因此不 import 任何 user 模块的实现包。
type authorStats struct {
	db    *gorm.DB
	apply AuthorStatsApplier
}

// ApplyArticleDelta 更新作者的累计文章数与浏览量。
func (a *authorStats) ApplyArticleDelta(ctx context.Context, authorID domain.UserID, articleDelta, viewDelta int) error {
	_ = ctx
	if a.apply == nil {
		// 没装配就跳过：统计是附属信息，不该让删文章因为"统计没接上"而失败。
		return nil
	}
	return a.apply(a.db, authorID.Uint64(), articleDelta, viewDelta)
}

// NewUserPort 构造只读用户端口。
//
// 导出是为了让集成测试能单独验证"作者信息 + 三项统计"的组装，
// 而不必为了一个只读查询把整个 Service 的依赖都装配起来。
func NewUserPort(db *gorm.DB) app.UserPort { return &userPort{db: db} }

// userPort 是 app.UserPort 的实现：只读用户展示信息。
type userPort struct{ db *gorm.DB }

// Profile 取单个用户。
func (u *userPort) Profile(ctx context.Context, id domain.UserID) (app.AuthorView, error) {
	m, err := u.one(ctx, fromUserID(id))
	if err != nil {
		return app.AuthorView{}, err
	}
	return m, nil
}

// Profiles 批量取用户。
func (u *userPort) Profiles(ctx context.Context, ids []domain.UserID) (map[domain.UserID]app.AuthorView, error) {
	out := make(map[domain.UserID]app.AuthorView, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, fromUserID(id))
	}
	var rows []userRow
	if err := u.db.WithContext(ctx).Where("id IN ?", raw).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, m := range rows {
		out[domain.UserID(m.ID)] = toAuthorView(m)
	}
	// 缺失的用户补空视图：列表页不能因为一个脏 author_id 就整页失败。
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = app.AuthorView{ID: id}
		}
	}
	return out, nil
}

func (u *userPort) one(ctx context.Context, id ctype.ID) (app.AuthorView, error) {
	var m userRow
	if err := u.db.WithContext(ctx).Take(&m, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return app.AuthorView{ID: domain.UserID(id)}, nil
		}
		return app.AuthorView{}, err
	}
	v := toAuthorView(m)

	// 统计缺失不是错误：新用户可能还没有 user_stat 行，此时保持零值即可。
	var st userStatRow
	if err := u.db.WithContext(ctx).Take(&st, "user_id = ?", id).Error; err == nil {
		v.ArticleCount = st.ArticleCount
		v.ArticleVisitedCount = st.ArticleVisitedCount
		v.FansCount = st.FansCount
	}
	return v, nil
}

func toAuthorView(m userRow) app.AuthorView {
	return app.AuthorView{
		ID:        domain.UserID(m.ID),
		Username:  m.Username,
		Nickname:  m.Nickname,
		Avatar:    m.Avatar,
		Abstract:  m.Abstract,
		CreatedAt: m.CreatedAt,
	}
}

// commentPort 是 app.CommentPort 的实现。
type commentPort struct{ db *gorm.DB }

// DeleteByArticle 删除某文章全部评论，返回条数。
//
// 先用 Count 再 Delete：审计日志与接口响应都要条数，
// 而 GORM 的 RowsAffected 在软删时并不等于"真实存在过的条数"（重复执行会是 0）。
func (c *commentPort) DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error) {
	q := c.db.WithContext(ctx).Model(&commentRow{}).
		Where("article_id = ?", fromArticleID(articleID))

	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if err := c.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&commentRow{}).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountByArticle 统计评论数。
func (c *commentPort) CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error) {
	var n int64
	if err := c.db.WithContext(ctx).Model(&commentRow{}).
		Where("article_id = ?", fromArticleID(articleID)).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// diggPort 是 app.DiggPort 的实现。
type diggPort struct{ db *gorm.DB }

// Exists 判断是否已点赞。
func (d *diggPort) Exists(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	var n int64
	if err := d.db.WithContext(ctx).Model(&articleDiggRow{}).
		Where("article_id = ? AND user_id = ?", fromArticleID(articleID), fromUserID(userID)).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Add 新增点赞；已存在返回 false。
func (d *diggPort) Add(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	exists, err := d.Exists(ctx, articleID, userID)
	if err != nil || exists {
		return false, err
	}
	return true, d.db.WithContext(ctx).Create(&articleDiggRow{
		ArticleID: fromArticleID(articleID),
		UserID:    fromUserID(userID),
	}).Error
}

// Remove 取消点赞；本不存在返回 false。
func (d *diggPort) Remove(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	exists, err := d.Exists(ctx, articleID, userID)
	if err != nil || !exists {
		return false, err
	}
	err = d.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ?", fromArticleID(articleID), fromUserID(userID)).
		Delete(&articleDiggRow{}).Error
	return err == nil, err
}

// DeleteByArticle 删除某文章全部点赞。
func (d *diggPort) DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error) {
	q := d.db.WithContext(ctx).Model(&articleDiggRow{}).
		Where("article_id = ?", fromArticleID(articleID))
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if err := d.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&articleDiggRow{}).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}
