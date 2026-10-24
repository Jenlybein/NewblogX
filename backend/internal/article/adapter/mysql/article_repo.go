package mysql

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// ArticleRepo 是 domain.Repository 的 MySQL 实现。
type ArticleRepo struct {
	db *gorm.DB
}

// NewArticleRepo 构造仓储。
func NewArticleRepo(db *gorm.DB) *ArticleRepo { return &ArticleRepo{db: db} }

// Find 按 ID 读取。
func (r *ArticleRepo) Find(ctx context.Context, id domain.ArticleID) (*domain.Article, error) {
	var m models.ArticleModel
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", fromArticleID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrArticleNotFound
		}
		return nil, err
	}
	tags, err := r.tagIDs(ctx, []ctype.ID{m.ID})
	if err != nil {
		return nil, err
	}
	return toDomainArticle(m, tags[m.ID]), nil
}

// FindMany 批量读取。
func (r *ArticleRepo) FindMany(ctx context.Context, ids []domain.ArticleID) ([]*domain.Article, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rawIDs := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		rawIDs = append(rawIDs, fromArticleID(id))
	}

	var rows []models.ArticleModel
	if err := r.db.WithContext(ctx).Where("id IN ?", rawIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// 保持入参顺序：调用方（如收藏列表）依赖它做展示排序。
	byID := make(map[ctype.ID]models.ArticleModel, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	tags, err := r.tagIDs(ctx, rawIDs)
	if err != nil {
		return nil, err
	}

	out := make([]*domain.Article, 0, len(rows))
	for _, id := range rawIDs {
		if row, ok := byID[id]; ok {
			out = append(out, toDomainArticle(row, tags[id]))
		}
	}
	return out, nil
}

// List 按条件分页查询。
func (r *ArticleRepo) List(ctx context.Context, f domain.Filter, p domain.Page) ([]*domain.Article, int64, error) {
	p = p.Normalize()
	q := r.applyFilter(r.db.WithContext(ctx).Model(&models.ArticleModel{}), f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	var rows []models.ArticleModel
	if err := q.Order("created_at desc, id desc").Offset(p.Offset).Limit(p.Limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	ids := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	tags, err := r.tagIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*domain.Article, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainArticle(row, tags[row.ID]))
	}
	return out, total, nil
}

// Create 插入。
func (r *ArticleRepo) Create(ctx context.Context, a *domain.Article) error {
	m := &models.ArticleModel{}
	applyArticleTo(a, m)
	return r.db.WithContext(ctx).Create(m).Error
}

// Update 保存变更。
//
// 用 Select 显式列出可更新列：GORM 的 Updates(struct) 会忽略零值，
// 导致"把 category_id 清空""把 submitted_at 置 NULL"这类操作丢失。
func (r *ArticleRepo) Update(ctx context.Context, a *domain.Article) error {
	m := &models.ArticleModel{}
	applyArticleTo(a, m)

	tx := r.db.WithContext(ctx).Model(&models.ArticleModel{}).
		Where("id = ?", m.ID).
		Select(
			"title", "abstract", "content", "category_id", "cover",
			"comments_toggle", "publish_status", "visibility_status",
			"submitted_at", "reviewed_at", "reviewed_by", "updated_at",
		).
		Updates(m)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return domain.ErrArticleNotFound
	}
	return nil
}

// SoftDelete 软删（GORM 看到 DeletedAt 字段会自动写 deleted_at）。
func (r *ArticleRepo) SoftDelete(ctx context.Context, id domain.ArticleID) error {
	tx := r.db.WithContext(ctx).Delete(&models.ArticleModel{}, "id = ?", fromArticleID(id))
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return domain.ErrArticleNotFound
	}
	return nil
}

// applyFilter 把领域 Filter 翻成 SQL 条件。
//
// OnlyPublic 用 (publish_status, visibility_status) 双条件而不是子查询：
// 与旧实现 IsPublicVisible() 的判定完全一致，走已有的联合索引。
func (r *ArticleRepo) applyFilter(q *gorm.DB, f domain.Filter) *gorm.DB {
	if f.OnlyPublic {
		q = q.Where("publish_status = ? AND visibility_status = ?",
			enum.ArticleStatusPublished, enum.ArticleVisibilityVisible)
	}
	if f.AuthorID != nil {
		q = q.Where("author_id = ?", fromUserID(*f.AuthorID))
	}
	if f.CategoryID != nil {
		q = q.Where("category_id = ?", uint64(*f.CategoryID))
	}
	if len(f.Statuses) > 0 {
		statuses := make([]enum.ArticleStatus, 0, len(f.Statuses))
		for _, s := range f.Statuses {
			statuses = append(statuses, fromStatus(s))
		}
		q = q.Where("publish_status IN ?", statuses)
	}
	if len(f.TagIDs) > 0 {
		ids := make([]ctype.ID, 0, len(f.TagIDs))
		for _, id := range f.TagIDs {
			ids = append(ids, ctype.ID(id))
		}
		q = q.Where("id IN (?)",
			r.db.Model(&models.ArticleTagModel{}).Select("article_id").Where("tag_id IN ?", ids))
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR abstract LIKE ?", like, like)
	}
	return q
}

// tagIDs 批量查询文章标签，返回 articleID → tagIDs。
func (r *ArticleRepo) tagIDs(ctx context.Context, ids []ctype.ID) (map[ctype.ID][]domain.TagID, error) {
	out := make(map[ctype.ID][]domain.TagID, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	// 用 Table 而不是 ArticleTagModel：这张表是 many2many 自动建出来的
	// 纯 join 表（只有 article_id / tag_id 两列，没有 deleted_at / id），
	// 用结构体查会被 GORM 加上 `deleted_at IS NULL` 而报 Unknown column。
	var links []struct {
		ArticleID ctype.ID
		TagID     ctype.ID
	}
	if err := r.db.WithContext(ctx).
		Table("article_tag_models").
		Select("article_id, tag_id").
		Where("article_id IN ?", ids).
		Find(&links).Error; err != nil {
		return nil, err
	}
	for _, l := range links {
		out[l.ArticleID] = append(out[l.ArticleID], domain.TagID(l.TagID))
	}
	return out, nil
}
