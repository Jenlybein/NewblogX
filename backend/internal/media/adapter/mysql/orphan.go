package mysql

import (
	"context"
	"strings"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/cdc/imageref"

	"gorm.io/gorm"
)

// OrphanStore 是 domain.OrphanStore 的 MySQL 实现（issue #139）。
//
// 它只读三处：image_models（图片行）、image_ref_models（引用账本），
// 以及四类真实引用源（文章正文/封面、轮播封面、收藏封面、用户头像）。
// 本文件里没有任何 INSERT/UPDATE/DELETE —— 清单票的只读性在这里落地。
type OrphanStore struct {
	db *gorm.DB
	// qiNiu 传函数：存储域名/前缀可以在后台改，而反解 object_key 依赖前缀。
	// 缓存旧值会让"真实引用"整体扫不到（表现为所有图都像孤儿）。
	qiNiu func() conf.QiNiu
}

var _ domain.OrphanStore = (*OrphanStore)(nil)

// OrphanStore 同时向真删链路提供"指向已删除文章"的保护读数（只读）。
var _ domain.OrphanPurgeGuard = (*OrphanStore)(nil)

// liveRefScanChunk 是重扫真实引用时的分页大小。
//
// 分页不是为了快，是为了内存：文章正文可能很大，一次 Find 全表会把所有
// 正文读进内存。扫完一页即弃，单次扫描的内存占用与总行数无关。
const liveRefScanChunk = 200

// NewOrphanStore 构造清单数据源。qiNiu 为空时按空配置反解（前缀回落 "images"）。
func NewOrphanStore(db *gorm.DB, qiNiu func() conf.QiNiu) *OrphanStore {
	if qiNiu == nil {
		qiNiu = func() conf.QiNiu { return conf.QiNiu{} }
	}
	return &OrphanStore{db: db, qiNiu: qiNiu}
}

// ListCandidates 实现 domain.OrphanStore。
func (s *OrphanStore) ListCandidates(ctx context.Context, spec domain.OrphanScanSpec) ([]domain.Image, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	// 状态白名单为空等于"一张都不许进"：宁可空清单，也不要静默退化成整表扫描。
	if len(spec.Statuses) == 0 || spec.Limit <= 0 {
		return nil, 0, nil
	}
	limit := spec.Limit
	if limit > domain.MaxOrphanBatchLimit {
		limit = domain.MaxOrphanBatchLimit
	}

	base := func() *gorm.DB {
		return s.db.WithContext(ctx).Model(&imageRow{}).
			Where("created_at < ?", spec.UploadedBefore).
			Where("status IN ?", statusValues(spec.Statuses))
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []imageRow
	// 老图先扫：清单是"该删的先看"，重复执行时同一批图不会被新图挤掉。
	if err := base().Order("created_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]domain.Image, 0, len(rows))
	for _, row := range rows {
		out = append(out, toImage(row))
	}
	return out, total, nil
}

// LedgerSnapshot 实现 domain.OrphanStore。
func (s *OrphanStore) LedgerSnapshot(ctx context.Context, imageIDs []domain.ImageID) (domain.LedgerSnapshot, error) {
	out := domain.LedgerSnapshot{
		Refs:    make(map[domain.ImageID]int, len(imageIDs)),
		Avatars: make(map[domain.ImageID]bool, len(imageIDs)),
	}
	if s.db == nil {
		return out, gorm.ErrInvalidDB
	}
	raw := toRawIDs(imageIDs)
	if len(raw) == 0 {
		return out, nil
	}

	// 账本行数：一次分组查询，不给每张图各发一条 SQL。
	var counts []struct {
		ImageID ctype.ID `gorm:"column:image_id"`
		Total   int64    `gorm:"column:total"`
	}
	if err := s.db.WithContext(ctx).Model(&imageRefRow{}).
		Select("image_id, COUNT(*) AS total").
		Where("image_id IN ?", raw).
		Group("image_id").
		Find(&counts).Error; err != nil {
		return out, err
	}
	for _, row := range counts {
		out.Refs[row.ImageID] = int(row.Total)
	}

	// 头像单独标出来：头像一律排除，不能只靠重扫（见 domain.LedgerSnapshot 的说明）。
	var avatars []struct {
		ImageID ctype.ID `gorm:"column:image_id"`
	}
	if err := s.db.WithContext(ctx).Model(&imageRefRow{}).
		Select("image_id").
		Where("image_id IN ? AND field = ?", raw, uint(image_ref_enum.RefFieldUserAvatar)).
		Group("image_id").
		Find(&avatars).Error; err != nil {
		return out, err
	}
	for _, row := range avatars {
		out.Avatars[row.ImageID] = true
	}
	return out, nil
}

// LiveRefs 实现 domain.OrphanStore：重扫四类引用源，只统计关心的对象键。
//
// 为什么不直接信任账本：账本由 CDC 异步维护、有死信；引用靠从 URL 反解
// object_key，URL 的域名/前缀与当前存储配置不匹配时会被丢弃。所以清单
// 的最终判定必须有一份独立于账本的读数。
func (s *OrphanStore) LiveRefs(ctx context.Context, objectKeys []string) (map[string]int, error) {
	wanted := make(map[string]struct{}, len(objectKeys))
	for _, key := range objectKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		wanted[key] = struct{}{}
	}
	out := make(map[string]int, len(wanted))
	if len(wanted) == 0 {
		return out, nil
	}
	if s.db == nil {
		return out, gorm.ErrInvalidDB
	}
	cfg := s.qiNiu()

	// 文章：正文 markdown 里的图（按出现次数计）+ 封面。
	if err := scanSource[articleRefSourceRow](ctx, s.db, []string{"id", "content", "cover"}, func(row *articleRefSourceRow) {
		bump(wanted, out, imageref.ExtractObjectKey(row.Cover, cfg))
		for _, rawURL := range imageref.ParseMarkdownImageURLs(row.Content) {
			bump(wanted, out, imageref.ExtractObjectKey(rawURL, cfg))
		}
	}); err != nil {
		return nil, err
	}

	// 用户头像。
	if err := scanSource[userRefSourceRow](ctx, s.db, []string{"id", "avatar"}, func(row *userRefSourceRow) {
		bump(wanted, out, imageref.ExtractObjectKey(row.Avatar, cfg))
	}); err != nil {
		return nil, err
	}

	// 轮播封面。
	if err := scanSource[bannerRefSourceRow](ctx, s.db, []string{"id", "cover"}, func(row *bannerRefSourceRow) {
		bump(wanted, out, imageref.ExtractObjectKey(row.Cover, cfg))
	}); err != nil {
		return nil, err
	}

	// 收藏夹封面。
	if err := scanSource[favoriteRefSourceRow](ctx, s.db, []string{"id", "cover"}, func(row *favoriteRefSourceRow) {
		bump(wanted, out, imageref.ExtractObjectKey(row.Cover, cfg))
	}); err != nil {
		return nil, err
	}

	return out, nil
}

// bump 只在关心的对象键上累加，其余的 URL 直接丢弃。
func bump(wanted map[string]struct{}, out map[string]int, objectKey string) {
	if objectKey == "" {
		return
	}
	if _, ok := wanted[objectKey]; !ok {
		return
	}
	out[objectKey]++
}

// DeletedArticleRefs 实现 domain.OrphanPurgeGuard。
//
// 只统计 ref_type=article 且所属文章已经软删（在回收站）的账本行。
// 这类行不能跟着图片一起删：用户恢复文章后，图已经没了就补不回来。
//
// join 必须显式写 articles.deleted_at IS NOT NULL —— GORM 的软删 scope
// 只作用于主表，不会自动加给 join 进来的表；写漏了会把活文章也算进来，
// 结果是"该删的一张都删不掉"。
func (s *OrphanStore) DeletedArticleRefs(ctx context.Context, imageIDs []domain.ImageID) (map[domain.ImageID]int, error) {
	out := make(map[domain.ImageID]int, len(imageIDs))
	raw := toRawIDs(imageIDs)
	if len(raw) == 0 {
		return out, nil
	}
	if s.db == nil {
		return out, gorm.ErrInvalidDB
	}

	var counts []struct {
		ImageID ctype.ID `gorm:"column:image_id"`
		Total   int64    `gorm:"column:total"`
	}
	if err := s.db.WithContext(ctx).Model(&imageRefRow{}).
		Select("image_id, COUNT(*) AS total").
		Joins("JOIN article_models AS articles ON articles.id = image_ref_models.owner_id AND articles.deleted_at IS NOT NULL").
		Where("image_ref_models.ref_type = ?", uint(image_ref_enum.RefTypeArticle)).
		Where("image_ref_models.image_id IN ?", raw).
		Group("image_id").
		Find(&counts).Error; err != nil {
		return out, err
	}
	for _, row := range counts {
		out[row.ImageID] = int(row.Total)
	}
	return out, nil
}

// statusValues 把状态枚举转成数据库里的整型值。
func statusValues(statuses []domain.Status) []uint {
	values := make([]uint, 0, len(statuses))
	for _, status := range statuses {
		values = append(values, uint(status))
	}
	return values
}

// ── 真实引用源的行 ────────────────────────────────────────────────

// articleRefSourceRow 是重扫文章时只读的列。
type articleRefSourceRow struct {
	ID        ctype.ID `gorm:"column:id"`
	Content   string   `gorm:"column:content"`
	Cover     string   `gorm:"column:cover"`
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (articleRefSourceRow) TableName() string { return "article_models" }

// sourceRowID 供分页游标使用。
func (r articleRefSourceRow) sourceRowID() uint64 { return r.ID.Uint64() }

// userRefSourceRow 是重扫用户头像时只读的列。
type userRefSourceRow struct {
	ID        ctype.ID `gorm:"column:id"`
	Avatar    string   `gorm:"column:avatar"`
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (userRefSourceRow) TableName() string { return "user_models" }

// sourceRowID 供分页游标使用。
func (r userRefSourceRow) sourceRowID() uint64 { return r.ID.Uint64() }

// bannerRefSourceRow 是重扫轮播封面时只读的列。
type bannerRefSourceRow struct {
	ID        ctype.ID `gorm:"column:id"`
	Cover     string   `gorm:"column:cover"`
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (bannerRefSourceRow) TableName() string { return "banner_models" }

// sourceRowID 供分页游标使用。
func (r bannerRefSourceRow) sourceRowID() uint64 { return r.ID.Uint64() }

// favoriteRefSourceRow 是重扫收藏封面时只读的列。
type favoriteRefSourceRow struct {
	ID        ctype.ID `gorm:"column:id"`
	Cover     string   `gorm:"column:cover"`
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (favoriteRefSourceRow) TableName() string { return "favorite_models" }

// sourceRowID 供分页游标使用。
func (r favoriteRefSourceRow) sourceRowID() uint64 { return r.ID.Uint64() }

// sourceRow 是"可按主键翻页扫描的业务表行"的最小约束。
type sourceRow interface {
	sourceRowID() uint64
}

// scanSource 按主键游标分页扫一张业务表，逐行交给 visit。
func scanSource[T any, PT interface {
	*T
	sourceRow
}](ctx context.Context, db *gorm.DB, columns []string, visit func(*T)) error {
	var cursor uint64
	for {
		var rows []T
		tx := db.WithContext(ctx).Select(columns).Order("id ASC").Limit(liveRefScanChunk)
		if cursor > 0 {
			tx = tx.Where("id > ?", cursor)
		}
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for index := range rows {
			visit(&rows[index])
			cursor = PT(&rows[index]).sourceRowID()
		}
		if len(rows) < liveRefScanChunk {
			return nil
		}
	}
}

// imageRefRow 是 image_ref_models 的行（本文件只读）。
//
// 带上 DeletedAt 是为了让 GORM 的默认 scope 自动补 `deleted_at IS NULL`：
// 账本写入端是物理删（Unscoped），但历史行不保证，少算一行等于把一张
// 还有引用的图当成孤儿。
type imageRefRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	ImageID   ctype.ID
	RefType   uint
	OwnerID   ctype.ID
	Field     uint
	Position  int
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (imageRefRow) TableName() string { return "image_ref_models" }
