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

// Save 新建或更新分类。
//
// 存在性显式判定，不看 RowsAffected：MySQL 对"值没变"的 UPDATE 返回 0 行变更，
// 把它当成"行不存在"就会去 INSERT，然后撞上自己那行、被翻成「名称重复」——
// 一次什么都没改的保存（个人主页弹窗只点了保存）就被拒了。
//
// 新建走平台的 RestoreOrCreateUnique（match = UserID + Title）：
// (user_id, title) 上是**不含** deleted_at 的唯一索引，而删除是软删，裸 INSERT 会让
// "删掉一个分类之后再建同名"永远得到 1062。房规（ADR-0036 约束 1）是恢复那条
// 软删行、保留原 ID，与点赞 / 标签一致。
func (r *CategoryRepo) Save(ctx context.Context, c *taxonomy.Category) error {
	id := ctype.ID(c.ID())
	return saveUniqueWithRestore(ctx, r.db, &categoryRow{}, id, &categoryRow{
		BaseRow: BaseRow{ID: id},
		Title:   c.Title(),
		UserID:  fromUserID(c.UserID()),
	}, map[string]any{
		"title":   c.Title(),
		"user_id": fromUserID(c.UserID()),
	}, []string{"UserID", "Title"})
}

// saveUniqueWithRestore 是"唯一索引 + 软删"实体在 Save 里的统一写法。
//
// 分类与标签的 Save 只差三样东西：写哪张表、匹配键是哪几列、恢复/更新时要写回
// 哪些字段；中间的判断是同一件事，此前在两个仓储里各写了一遍（连
// `liveExists` 与"1062 翻译"都是两份），一处修另一处漏（标签侧就漏过）。
//
// 三条语义：
//  1. **活行存在就走 UPDATE**，不看 RowsAffected（理由见两个 Save 的注释）；
//  2. **活行不存在就交给平台助手**：先恢复同名软删行（保留原 ID），再插入；
//  3. 助手返回 `false` 表示"没恢复、插入又撞了唯一键"——对调用方就是
//     「名称重复」。把它当成功吞掉，用户会看到"保存成功"但库里什么都没有。
//
// `table` 只用来定位表与软删作用域（传零值行即可）；`row` 是这次要写入的字段
// （恢复与插入共用），`update` 是命中活行时更新的列。
func saveUniqueWithRestore[T any](
	ctx context.Context,
	db *gorm.DB,
	table any,
	id ctype.ID,
	row *T,
	update map[string]any,
	match []string,
) error {
	var live int64
	if err := db.WithContext(ctx).Model(table).Where("id = ?", id).Count(&live).Error; err != nil {
		return err
	}
	if live == 0 {
		restored, err := platformmysql.RestoreOrCreateUnique(db.WithContext(ctx), row, match)
		if err != nil {
			return taxonomySaveError(err)
		}
		if !restored {
			return taxonomy.ErrTitleDuplicated
		}
		return nil
	}
	return taxonomySaveError(db.WithContext(ctx).Model(table).
		Where("id = ?", id).
		Updates(update).Error)
}

// taxonomySaveError 把底层唯一键冲突翻译成领域错误（分类与标签共用）。
//
// 应用层虽然先做了 ExistsTitle 查重，但两个并发请求仍可能同时通过查重，
// 最后由唯一索引（分类 `uk_category_user_title` / 标签 `idx_tag_models_title`）
// 拒绝后一次写入。如果不在这里翻译，handler 会把 gorm.ErrDuplicatedKey 原样透传，
// 用户看到的会是 "duplicated key not allowed" / MySQL 1062，
// 而不是 apperr 消息表里的 "名称重复"。
func taxonomySaveError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return taxonomy.ErrTitleDuplicated
	}
	return err
}

// Delete 删除分类。
//
// 两步，顺序不得颠倒（ADR-0036 约束 3）：
//  1. 先把挂在该分类下的文章 `category_id` 置空 —— 那列上有外键指向分类表，
//     但删除是软删，外键永远不会触发；不显式置空，文章就会继续指着一个
//     已经"不存在"的分类（详情页仍显示它、列表按它筛出空结果）。
//  2. 再软删分类本身。
//
// 两步共用调用方的事务句柄（UseCase 的 WithinTx 现场构造本仓储），
// 因此中途失败不会留下"文章已解绑、分类还在"或反过来的半截状态。
// 置空会经 binlog 触发搜索投影重算（ADR-0009），不需要补偿任务。
//
// 两条容易写错的边界（ADR-0036 决策 6 / 已知限制）：
//   - **回收站里的文章保留分类归属**：软删作用域让它们不参与这次置空，这是有意的。
//     删除是软删、同名重建会恢复同一行分类，归属随之回来；顺手清掉反而丢信息。
//   - **不写 `updated_at`**：用 `UpdateColumn` 而不是 `Updates`。后者会顺带把
//     updated_at 刷成现在（GORM 的自动时间戳），于是所有原属该分类的文章在
//     详情页上集体显示一个新的"更新于"——用户只是删了个分类，文章并没改。
func (r *CategoryRepo) Delete(ctx context.Context, id domain.CategoryID) error {
	if err := r.db.WithContext(ctx).Model(&articleRow{}).
		Where("category_id = ?", ctype.ID(id)).
		UpdateColumn("category_id", nil).Error; err != nil {
		return err
	}
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

// Save 新建或更新标签（判断形状与分类共用 saveUniqueWithRestore）。
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
	return saveUniqueWithRestore(ctx, r.db, &tagRow{}, id, &tagRow{
		BaseRow:     BaseRow{ID: id},
		Title:       t.Title(),
		Sort:        t.Sort(),
		Description: t.Description(),
		IsEnabled:   t.Enabled(),
		CreatedBy:   fromUserID(t.CreatedBy()),
	}, map[string]any{
		"title":       t.Title(),
		"sort":        t.Sort(),
		"description": t.Description(),
		"is_enabled":  t.Enabled(),
	}, []string{"Title"})
}

// Delete 软删标签。
//
// 关联表不在这里清理：删除门禁（Service.DeleteTags）已经保证被引用时不会走到这，
// 所以能删的标签本来就没有关联行。
func (r *TagRepo) Delete(ctx context.Context, id domain.TagID) error {
	return r.db.WithContext(ctx).Delete(&tagRow{}, "id = ?", ctype.ID(id)).Error
}

// ListIDs 返回全部未软删标签的 ID（换序的集合校验用）。
//
// 与 List 的读口径一致：**包含停用项**——后台网格展示的是词库全集，提交的
// 完整序列必须与这个集合相等。集合校验只比成员、不比顺序，所以这里不排序：
// 多一个 ORDER BY 只会让人以为顺序有意义。
func (r *TagRepo) ListIDs(ctx context.Context) ([]domain.TagID, error) {
	var raw []ctype.ID
	err := r.db.WithContext(ctx).Model(&tagRow{}).
		Pluck("id", &raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.TagID, 0, len(raw))
	for _, v := range raw {
		out = append(out, domain.TagID(v))
	}
	return out, nil
}

// UpdateSortOrders 把给定顺序落成 sort 值。
//
// 列表读面是 `ORDER BY sort desc, id asc`，所以第 0 个（网格最靠前）拿最大的
// sort：n、n-1 … 1。逐条 UPDATE 足够（标签以百计），不折腾 CASE WHEN。
// 集合一致性由 app 层的 taxonomy.CheckSortOrder 保证，这里只负责落库。
func (r *TagRepo) UpdateSortOrders(ctx context.Context, ids []domain.TagID) error {
	db := r.db.WithContext(ctx)
	n := len(ids)
	for i, id := range ids {
		if err := db.Model(&tagRow{}).
			Where("id = ?", ctype.ID(id)).
			Update("sort", n-i).Error; err != nil {
			return err
		}
	}
	return nil
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
