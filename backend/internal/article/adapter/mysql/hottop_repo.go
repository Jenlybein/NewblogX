package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/hottop"
	"myblogx/internal/kernel/ctype"
)

// HotTopRepo 是 hottop.Repository 的 MySQL 实现。
//
// 与 TopRepo（作者置顶）刻意不共用任何方法：唯一约束不同（这里只有 article_id）、
// 有序号要维护、失效判定要 join 文章表。共用会让两个功能互相牵制。
type HotTopRepo struct {
	db *gorm.DB
}

// NewHotTopRepo 构造仓储。
func NewHotTopRepo(db *gorm.DB) *HotTopRepo { return &HotTopRepo{db: db} }

// Save 新增置顶，返回是否真的新增。
//
// 三种情况：
//   - 已在置顶（未软删）：幂等返回 false，**不改序号**（重复设置不该把它挪到末尾）；
//   - 曾经取消（行被软删）：恢复那一行并按新序号排到末尾——
//     article_id 上有唯一索引，软删行仍占着它，重新 INSERT 会撞 1062；
//   - 从未置顶：插入新行，序号 = 当前最大 + 1（新置顶排末尾）。
func (r *HotTopRepo) Save(ctx context.Context, articleID domain.ArticleID) (bool, error) {
	db := r.db.WithContext(ctx)

	var row articleHotTopRow
	err := db.Unscoped().
		Where("article_id = ?", fromArticleID(articleID)).
		Take(&row).Error
	switch {
	case err == nil && !row.DeletedAt.Valid:
		return false, nil
	case err == nil:
		next, err := r.nextSortOrder(db)
		if err != nil {
			return false, err
		}
		err = db.Unscoped().Model(&articleHotTopRow{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{"deleted_at": nil, "sort_order": next}).Error
		return err == nil, err
	case errors.Is(err, gorm.ErrRecordNotFound):
		next, err := r.nextSortOrder(db)
		if err != nil {
			return false, err
		}
		return true, db.Create(&articleHotTopRow{
			ArticleID: fromArticleID(articleID),
			SortOrder: next,
		}).Error
	default:
		return false, err
	}
}

// Delete 取消置顶（软删）并压实剩余序号。
//
// 用软删而不是物理删：零 DDL 口径下这张表的结构就是"带 deleted_at 的关系表"，
// 而且软删保留了"曾经置顶过"的痕迹，排查运营操作时有用。
func (r *HotTopRepo) Delete(ctx context.Context, articleID domain.ArticleID) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("article_id = ?", fromArticleID(articleID)).
		Delete(&articleHotTopRow{}).Error; err != nil {
		return err
	}
	return r.compact(db)
}

// Exists 判断是否已置顶。
func (r *HotTopRepo) Exists(ctx context.Context, articleID domain.ArticleID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&articleHotTopRow{}).
		Where("article_id = ?", fromArticleID(articleID)).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Count 统计当前置顶条数（不含软删行）。
func (r *HotTopRepo) Count(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&articleHotTopRow{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// ListIDs 按序号返回当前置顶的文章 ID。
func (r *HotTopRepo) ListIDs(ctx context.Context) ([]domain.ArticleID, error) {
	var raw []ctype.ID
	if err := r.db.WithContext(ctx).Model(&articleHotTopRow{}).
		Order("sort_order asc, id asc").
		Pluck("article_id", &raw).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ArticleID, 0, len(raw))
	for _, id := range raw {
		out = append(out, toArticleID(id))
	}
	return out, nil
}

// ListCards 列出仍公开的置顶卡片。
//
// 失效行在这里被 JOIN 过滤掉（ADR-0017 决策 5：首页不展示、后台仍可见）：
//   - 文章软删（a.deleted_at 非空）→ 过滤；
//   - 不再是"已发布 + visible" → 过滤。
//
// 过滤放在 SQL 而不是 Go：置顶最多 12 条，但首页每次请求都读，
// 且"公开可见"的口径必须与 domain.CanBePinned 一致，写在一处更好对齐。
func (r *HotTopRepo) ListCards(ctx context.Context) ([]hottop.Card, error) {
	type row struct {
		ArticleID      ctype.ID
		Title          string
		Abstract       string
		Cover          string
		ViewCount      int
		AuthorNickname string
		CategoryTitle  string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("article_hot_top_models AS t").
		Select("a.id AS article_id, a.title, a.abstract, a.cover, a.view_count, "+
			"COALESCE(u.nickname, '') AS author_nickname, COALESCE(c.title, '') AS category_title").
		Joins("JOIN article_models AS a ON a.id = t.article_id AND a.deleted_at IS NULL").
		Joins("LEFT JOIN user_models AS u ON u.id = a.author_id").
		Joins("LEFT JOIN category_models AS c ON c.id = a.category_id").
		Where("t.deleted_at IS NULL AND a.publish_status = ? AND a.visibility_status = ?",
			statusPublished, visibilityVisible).
		Order("t.sort_order ASC, t.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]hottop.Card, 0, len(rows))
	for _, m := range rows {
		out = append(out, hottop.Card{
			ArticleID:      toArticleID(m.ArticleID),
			Title:          m.Title,
			Abstract:       m.Abstract,
			Cover:          m.Cover,
			ViewCount:      m.ViewCount,
			AuthorNickname: m.AuthorNickname,
			CategoryTitle:  m.CategoryTitle,
		})
	}
	return out, nil
}

// ListAdminItems 列出全部置顶行（含已失效），供后台排序与清理。
//
// LEFT JOIN：文章被软删时行还在（能看见标题与状态），
// 文章被物理删掉时 a.id 为 NULL —— 此时 Article 置 nil，由 domain 判为失效。
func (r *HotTopRepo) ListAdminItems(ctx context.Context) ([]hottop.AdminItem, error) {
	type row struct {
		ArticleID    ctype.ID
		SortOrder    int
		Title        string
		Cover        string
		Found        bool
		Deleted      bool
		PublishState *int8
		Visibility   *string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("article_hot_top_models AS t").
		Select("t.article_id, t.sort_order, COALESCE(a.title, '') AS title, COALESCE(a.cover, '') AS cover, " +
			"a.id IS NOT NULL AS found, a.deleted_at IS NOT NULL AS deleted, " +
			"a.publish_status AS publish_state, a.visibility_status AS visibility").
		Joins("LEFT JOIN article_models AS a ON a.id = t.article_id").
		Where("t.deleted_at IS NULL").
		Order("t.sort_order ASC, t.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]hottop.AdminItem, 0, len(rows))
	for _, m := range rows {
		item := hottop.AdminItem{
			ArticleID: toArticleID(m.ArticleID),
			SortOrder: m.SortOrder,
			Title:     m.Title,
			Cover:     m.Cover,
		}
		if m.Found {
			state := &hottop.ArticleState{Deleted: m.Deleted}
			if m.PublishState != nil {
				state.Status = toStatus(*m.PublishState)
			}
			if m.Visibility != nil {
				state.Visibility = toVisibility(*m.Visibility)
			}
			item.Article = state
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateSortOrder 按给定顺序把序号改成 1..n。
//
// 集合一致性由 app 层的 hottop.CheckOrder 保证，这里只负责落库；
// 调换顺序时逐条 UPDATE 足够（上限 12 条），不必折腾 CASE WHEN。
func (r *HotTopRepo) UpdateSortOrder(ctx context.Context, ids []domain.ArticleID) error {
	db := r.db.WithContext(ctx)
	for i, id := range ids {
		if err := db.Model(&articleHotTopRow{}).
			Where("article_id = ?", fromArticleID(id)).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// nextSortOrder 返回下一个可用序号（当前最大 + 1，空表为 1）。
func (r *HotTopRepo) nextSortOrder(db *gorm.DB) (int, error) {
	var max int
	if err := db.Model(&articleHotTopRow{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&max).Error; err != nil {
		return 0, err
	}
	return max + 1, nil
}

// compact 把剩余置顶行的序号压实成 1..n。
//
// 为什么需要：序号会在后台页展示、也会被"上移/下移"当作位置基准；
// 取消一条后留个洞，人看到的就是"1,2,4"。
func (r *HotTopRepo) compact(db *gorm.DB) error {
	var rows []articleHotTopRow
	if err := db.Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return err
	}
	for i, row := range rows {
		if row.SortOrder == i+1 {
			continue
		}
		if err := db.Model(&articleHotTopRow{}).
			Where("id = ?", row.ID).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}
