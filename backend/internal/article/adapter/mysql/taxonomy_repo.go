package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"
	platformmysql "myblogx/internal/platform/mysql"
)

// CategoryRepo 是 taxonomy.CategoryRepository 的 MySQL 实现。
type CategoryRepo struct {
	db *gorm.DB
}

// NewCategoryRepo 构造仓储。
func NewCategoryRepo(db *gorm.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// Find 读取分类。
func (r *CategoryRepo) Find(ctx context.Context, id domain.CategoryID) (*taxonomy.Category, error) {
	var m categoryRow
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.ID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, taxonomy.ErrCategoryNotFound
		}
		return nil, err
	}
	return taxonomy.RestoreCategory(domain.CategoryID(m.ID), domain.UserID(m.UserID), m.Title), nil
}

// ListByUser 列出某用户的分类。
//
// 一次查询带回列表页需要的读模型字段：文章数、所属用户昵称/头像。
// 不用 Preload("ArticleList")：那会把整篇文章行都捞上来，只为算个 count。
func (r *CategoryRepo) ListByUser(ctx context.Context, userID domain.UserID) ([]*taxonomy.Category, error) {
	var rows []struct {
		ID           ctype.ID
		CreatedAt    time.Time
		UpdatedAt    time.Time
		UserID       ctype.ID
		Title        string
		ArticleCount int
		Nickname     string
		Avatar       string
	}
	if err := r.db.WithContext(ctx).
		Table("category_models AS c").
		Select("c.*, "+
			"(SELECT COUNT(*) FROM article_models a WHERE a.category_id = c.id AND a.deleted_at IS NULL) AS article_count, "+
			"u.nickname AS nickname, u.avatar AS avatar").
		Joins("LEFT JOIN user_models u ON u.id = c.user_id").
		Where("c.user_id = ? AND c.deleted_at IS NULL", fromUserID(userID)).
		Order("c.id asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*taxonomy.Category, 0, len(rows))
	for _, m := range rows {
		out = append(out, taxonomy.RestoreCategoryView(taxonomy.RestoreCategoryParams{
			ID:            domain.CategoryID(m.ID),
			UserID:        domain.UserID(m.UserID),
			Title:         m.Title,
			CreatedAt:     m.CreatedAt,
			UpdatedAt:     m.UpdatedAt,
			ArticleCount:  m.ArticleCount,
			OwnerNickname: m.Nickname,
			OwnerAvatar:   m.Avatar,
		}))
	}
	return out, nil
}

// ExistsTitle 判断同用户下名称是否重复。
func (r *CategoryRepo) ExistsTitle(ctx context.Context, userID domain.UserID, title string, excludeID domain.CategoryID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&categoryRow{}).
		Where("user_id = ? AND title = ?", fromUserID(userID), title)
	if !excludeID.IsZero() {
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
	updates := r.db.WithContext(ctx).Model(&categoryRow{}).
		Where("id = ?", id).
		Updates(map[string]any{"title": c.Title(), "user_id": fromUserID(c.UserID())})
	if updates.Error != nil {
		return categorySaveError(updates.Error)
	}
	if updates.RowsAffected > 0 {
		return nil
	}
	return categorySaveError(r.db.WithContext(ctx).Create(&categoryRow{
		BaseRow: BaseRow{ID: id},
		Title:   c.Title(),
		UserID:  fromUserID(c.UserID()),
	}).Error)
}

// categorySaveError 把底层唯一键冲突翻译成领域错误。
//
// 应用层虽然先做了 ExistsTitle 查重，但两个并发请求仍可能同时通过查重，
// 最后由 (user_id, title) 上的 uk_category_user_title 拒绝后一次写入。
// 如果不在这里翻译，handler 会把 gorm.ErrDuplicatedKey 原样透传，
// 用户看到的会是 "duplicated key not allowed" / MySQL 1062，
// 而不是 apperr 消息表里的 "名称重复"。
func categorySaveError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return taxonomy.ErrTitleDuplicated
	}
	return err
}

// Delete 删除分类。
func (r *CategoryRepo) Delete(ctx context.Context, id domain.CategoryID) error {
	return r.db.WithContext(ctx).Delete(&categoryRow{}, "id = ?", ctype.ID(id)).Error
}

// TagRepo 是 taxonomy.TagRepository 的 MySQL 实现。
type TagRepo struct {
	db *gorm.DB
}

// NewTagRepo 构造仓储。
func NewTagRepo(db *gorm.DB) *TagRepo { return &TagRepo{db: db} }

// Find 读取标签。
func (r *TagRepo) Find(ctx context.Context, id domain.TagID) (*taxonomy.Tag, error) {
	var m tagRow
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
	var rows []tagRow
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
//
// 文章数按关联表实时统计（子查询），不读 tag_models.article_count：
// 那一列没有任何维护者，真实数据里恒为 0；后台标签页与删除门禁必须看到同一个数。
func (r *TagRepo) List(ctx context.Context, includeDisabled bool) ([]*taxonomy.Tag, error) {
	// 用 Table 而不是 Model：这里的 select 带聚合子查询，且软删条件要显式写
	// （Table 不会自动补 deleted_at IS NULL）。
	q := r.db.WithContext(ctx).
		Table("tag_models AS t").
		Select("t.*, " + tagArticleRefCountSQL("t.id") + " AS article_count").
		Where("t.deleted_at IS NULL")
	if !includeDisabled {
		q = q.Where("t.is_enabled = ?", true)
	}
	var rows []tagRow
	if err := q.Order("t.sort desc, t.id asc").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*taxonomy.Tag, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTag(m))
	}
	return out, nil
}

// tagArticleRefCountSQL 是"该标签当前被多少篇文章引用"的实时统计子查询。
//
// 只把没被软删的文章算进去：文章删了链接行会留下（关联表是硬删，但删除路径
// 不做级联清理），把已删文章的链接算成引用会让标签永远删不掉。
// 关联表的 deleted_at 列在各环境并不一致，所以刻意不筛它（见 ReplaceArticleTags）。
func tagArticleRefCountSQL(tagIDColumn string) string {
	return "(SELECT COUNT(*) FROM article_tag_models atg " +
		"JOIN article_models a ON a.id = atg.article_id " +
		"WHERE atg.tag_id = " + tagIDColumn + " AND a.deleted_at IS NULL)"
}

// CountArticleRefs 统计仍引用该标签的文章数。
func (r *TagRepo) CountArticleRefs(ctx context.Context, id domain.TagID) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Table("article_tag_models AS atg").
		Joins("JOIN article_models AS a ON a.id = atg.article_id").
		Where("atg.tag_id = ? AND a.deleted_at IS NULL", ctype.ID(id)).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// ExistsTitle 判断标签名是否重复。
func (r *TagRepo) ExistsTitle(ctx context.Context, title string, excludeID domain.TagID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&tagRow{}).Where("title = ?", title)
	if !excludeID.IsZero() {
		q = q.Where("id <> ?", ctype.ID(excludeID))
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Save 新建或更新标签。
//
// 存在性显式判定，不看 RowsAffected：MySQL 对"值没变"的 UPDATE 返回 0 行变更，
// 把它当成"行不存在"就会去 INSERT，然后撞上自己那行、被翻成「名称重复」——
// 一次什么都没改的编辑（后台弹窗只点了保存）就被拒了。
//
// 新建走平台的 RestoreOrCreateUnique（match = Title）：title 上是**不含**
// deleted_at 的唯一索引，而删除是软删，裸 INSERT 会让"删掉一个零引用标签
// 之后再建同名"永远得到 1062。房规与点赞 / 收藏一致：软删行视为
// "这个名字以前存在过"，恢复它（保留原 ID），不再新建。
func (r *TagRepo) Save(ctx context.Context, t *taxonomy.Tag) error {
	id := ctype.ID(t.ID())
	live, err := r.liveExists(ctx, id)
	if err != nil {
		return err
	}
	if !live {
		_, err := platformmysql.RestoreOrCreateUnique(r.db.WithContext(ctx), &tagRow{
			BaseRow:     BaseRow{ID: id},
			Title:       t.Title(),
			Sort:        t.Sort(),
			Description: t.Description(),
			IsEnabled:   t.Enabled(),
			CreatedBy:   fromUserID(t.CreatedBy()),
		}, []string{"Title"})
		return tagSaveError(err)
	}
	return tagSaveError(r.db.WithContext(ctx).Model(&tagRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"title":       t.Title(),
			"sort":        t.Sort(),
			"description": t.Description(),
			"is_enabled":  t.Enabled(),
		}).Error)
}

// liveExists 判断活行（未软删）是否存在。
func (r *TagRepo) liveExists(ctx context.Context, id ctype.ID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&tagRow{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// tagSaveError 把底层唯一键冲突翻译成领域错误（与 categorySaveError 同一口径）。
//
// 应用层先做了 ExistsTitle 查重，但并发请求仍可能同时通过查重、最后由
// idx_tag_models_title 拒绝后一次写入。不翻译的话，管理员看到的是
// "duplicated key not allowed" / MySQL 1062，而不是「名称重复」。
func tagSaveError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return taxonomy.ErrTitleDuplicated
	}
	return err
}

// Delete 软删标签。
//
// 关联表不在这里清理：删除门禁（Service.DeleteTags）已经保证被引用时不会走到这，
// 所以能删的标签本来就没有关联行。
func (r *TagRepo) Delete(ctx context.Context, id domain.TagID) error {
	return r.db.WithContext(ctx).Delete(&tagRow{}, "id = ?", ctype.ID(id)).Error
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
		pk, err := idgen.Next()
		if err != nil {
			return err
		}
		rows = append(rows, map[string]any{
			"id":         pk.Uint64(),
			"article_id": aid.Uint64(),
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
func toDomainTag(m tagRow) *taxonomy.Tag {
	return taxonomy.RestoreTag(taxonomy.RestoreTagParams{
		ID:           domain.TagID(m.ID),
		Title:        m.Title,
		Sort:         m.Sort,
		Description:  m.Description,
		ArticleCount: m.ArticleCount,
		Enabled:      m.IsEnabled,
		CreatedBy:    domain.UserID(m.CreatedBy),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	})
}
