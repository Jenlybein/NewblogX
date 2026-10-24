package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/db_service"
)

// CategoryRepo 是 taxonomy.CategoryRepository 的 MySQL 实现。
type CategoryRepo struct {
	db *gorm.DB
}

// NewCategoryRepo 构造仓储。
func NewCategoryRepo(db *gorm.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// Find 读取分类。
func (r *CategoryRepo) Find(ctx context.Context, id domain.CategoryID) (*taxonomy.Category, error) {
	var m models.CategoryModel
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.ID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, taxonomy.ErrCategoryNotFound
		}
		return nil, err
	}
	return taxonomy.RestoreCategory(domain.CategoryID(m.ID), domain.UserID(m.UserID), m.Title), nil
}

// ListByUser 列出某用户的分类。
func (r *CategoryRepo) ListByUser(ctx context.Context, userID domain.UserID) ([]*taxonomy.Category, error) {
	var rows []models.CategoryModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", fromUserID(userID)).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*taxonomy.Category, 0, len(rows))
	for _, m := range rows {
		out = append(out, taxonomy.RestoreCategory(domain.CategoryID(m.ID), domain.UserID(m.UserID), m.Title))
	}
	return out, nil
}

// ExistsTitle 判断同用户下名称是否重复。
func (r *CategoryRepo) ExistsTitle(ctx context.Context, userID domain.UserID, title string, excludeID domain.CategoryID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&models.CategoryModel{}).
		Where("user_id = ? AND title = ?", fromUserID(userID), title)
	if excludeID != 0 {
		q = q.Where("id <> ?", ctype.ID(excludeID))
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Save 新建或更新（upsert）。
//
// 先按 ID 更新，影响 0 行说明库里没有，再插入。不用 GORM 的 Save：
// 它会用主键判断存在性并可能触发全字段覆盖，对软删表容易踩坑。
func (r *CategoryRepo) Save(ctx context.Context, c *taxonomy.Category) error {
	id := ctype.ID(c.ID())
	updates := r.db.WithContext(ctx).Model(&models.CategoryModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"title": c.Title(), "user_id": fromUserID(c.UserID())})
	if updates.Error != nil {
		return updates.Error
	}
	if updates.RowsAffected > 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&models.CategoryModel{
		Model:  models.Model{ID: id},
		Title:  c.Title(),
		UserID: fromUserID(c.UserID()),
	}).Error
}

// Delete 删除分类。
func (r *CategoryRepo) Delete(ctx context.Context, id domain.CategoryID) error {
	return r.db.WithContext(ctx).Delete(&models.CategoryModel{}, "id = ?", ctype.ID(id)).Error
}

// TagRepo 是 taxonomy.TagRepository 的 MySQL 实现。
type TagRepo struct {
	db *gorm.DB
}

// NewTagRepo 构造仓储。
func NewTagRepo(db *gorm.DB) *TagRepo { return &TagRepo{db: db} }

// Find 读取标签。
func (r *TagRepo) Find(ctx context.Context, id domain.TagID) (*taxonomy.Tag, error) {
	var m models.TagModel
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.ID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, taxonomy.ErrTagNotFound
		}
		return nil, err
	}
	return toDomainTag(m), nil
}

// FindMany 批量读取。
func (r *TagRepo) FindMany(ctx context.Context, ids []domain.TagID) ([]*taxonomy.Tag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, ctype.ID(id))
	}
	var rows []models.TagModel
	if err := r.db.WithContext(ctx).Where("id IN ?", raw).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*taxonomy.Tag, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTag(m))
	}
	return out, nil
}

// List 列出标签。
func (r *TagRepo) List(ctx context.Context, includeDisabled bool) ([]*taxonomy.Tag, error) {
	q := r.db.WithContext(ctx).Model(&models.TagModel{})
	if !includeDisabled {
		q = q.Where("is_enabled = ?", true)
	}
	var rows []models.TagModel
	if err := q.Order("sort desc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*taxonomy.Tag, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTag(m))
	}
	return out, nil
}

// ExistsTitle 判断标签名是否重复。
func (r *TagRepo) ExistsTitle(ctx context.Context, title string, excludeID domain.TagID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&models.TagModel{}).Where("title = ?", title)
	if excludeID != 0 {
		q = q.Where("id <> ?", ctype.ID(excludeID))
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Save 新建或更新标签（upsert，语义同分类）。
func (r *TagRepo) Save(ctx context.Context, t *taxonomy.Tag) error {
	id := ctype.ID(t.ID())
	updates := r.db.WithContext(ctx).Model(&models.TagModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"title":       t.Title(),
			"sort":        t.Sort(),
			"description": t.Description(),
			"is_enabled":  t.Enabled(),
		})
	if updates.Error != nil {
		return updates.Error
	}
	if updates.RowsAffected > 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&models.TagModel{
		Model:       models.Model{ID: id},
		Title:       t.Title(),
		Sort:        t.Sort(),
		Description: t.Description(),
		IsEnabled:   t.Enabled(),
		CreatedBy:   fromUserID(t.CreatedBy()),
	}).Error
}

// Delete 删除标签及关联（TagModel 自带 BeforeDelete 清理 article_tag）。
func (r *TagRepo) Delete(ctx context.Context, id domain.TagID) error {
	return r.db.WithContext(ctx).Delete(&models.TagModel{}, "id = ?", ctype.ID(id)).Error
}

// ReplaceArticleTags 用给定集合替换文章标签。
//
// 关系表是 many2many 自动建的纯 join 表（无 id / 无 deleted_at），
// 因此：① 用 Table 操作，② 删除是**硬删**——
// 若走软删语义，同一对 (article_id, tag_id) 再次添加会撞唯一键。
func (r *TagRepo) ReplaceArticleTags(ctx context.Context, articleID domain.ArticleID, tagIDs []domain.TagID) error {
	aid := fromArticleID(articleID)
	if err := r.db.WithContext(ctx).
		Table("article_tag_models").
		Where("article_id = ?", aid).
		Delete(nil).Error; err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	// 显式给主键：这张表是 many2many 关系表，`id` 是 `autoIncrement:false`
	// 的雪花主键，没有数据库默认值；不给就会报 Field 'id' doesn't have a default value。
	// 同时刻意不写 deleted_at——不同环境里这张表是否带该列并不一致（见 baseline）。
	rows := make([]map[string]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		pk, err := db_service.NextSnowflakeID()
		if err != nil {
			return err
		}
		rows = append(rows, map[string]any{
			"id":         uint64(pk),
			"article_id": uint64(aid),
			"tag_id":     id.Uint64(),
		})
	}
	return r.db.WithContext(ctx).Table("article_tag_models").Create(&rows).Error
}

// ArticleTagIDs 读取文章标签。
func (r *TagRepo) ArticleTagIDs(ctx context.Context, articleID domain.ArticleID) ([]domain.TagID, error) {
	var links []struct {
		TagID ctype.ID
	}
	if err := r.db.WithContext(ctx).
		Table("article_tag_models").
		Select("tag_id").
		Where("article_id = ?", fromArticleID(articleID)).
		Find(&links).Error; err != nil {
		return nil, err
	}
	out := make([]domain.TagID, 0, len(links))
	for _, l := range links {
		out = append(out, domain.TagID(l.TagID))
	}
	return out, nil
}

// toDomainTag 把 row 转成领域标签。
func toDomainTag(m models.TagModel) *taxonomy.Tag {
	return taxonomy.RestoreTag(taxonomy.RestoreTagParams{
		ID:           domain.TagID(m.ID),
		Title:        m.Title,
		Sort:         m.Sort,
		Description:  m.Description,
		ArticleCount: m.ArticleCount,
		Enabled:      m.IsEnabled,
		CreatedBy:    domain.UserID(m.CreatedBy),
	})
}
