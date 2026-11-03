package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// CollectionRepo 是 collection.Repository 的 MySQL 实现。
//
// 与 HotTopRepo（热门置顶）刻意不共用任何方法：合集有标题/描述/封面/展示开关，
// 且收录条数没有上限。共用会让两个功能互相牵制。
type CollectionRepo struct {
	db *gorm.DB
}

// NewCollectionRepo 构造仓储。
func NewCollectionRepo(db *gorm.DB) *CollectionRepo { return &CollectionRepo{db: db} }

// Create 插入合集。
//
// 用 map 插入而不是结构体：is_display 带 default:1，GORM 在 Create 时会**跳过
// 零值字段**让数据库默认值生效——于是"建一个不展示的合集"会被默默写成展示。
// 这正是"先准备、后公布"的运营节奏会踩的坑（Select("*") 也绕不过去）。
func (r *CollectionRepo) Create(ctx context.Context, c *collection.Collection) error {
	return r.db.WithContext(ctx).Model(&collectionRow{}).Create(map[string]any{
		"id":          ctype.ID(c.ID()),
		"created_at":  c.CreatedAt(),
		"updated_at":  c.UpdatedAt(),
		"title":       c.Title(),
		"description": c.Description(),
		"cover":       c.Cover(),
		"is_display":  c.Displayed(),
		"sort_order":  c.SortOrder(),
		"creator_id":  ctype.ID(c.CreatorID()),
	}).Error
}

// Update 保存已有合集的变更。
//
// 用 map 而不是结构体做 Updates：GORM 对结构体会跳过零值字段
// （is_display=false、sort_order=0 就永远改不动），而"关闭展示"正是把 true 改成 false。
//
// 软删也走这里：域里的 SoftDelete 只改聚合状态，落库必须显式写 deleted_at
// （GORM 的默认作用域会把 WHERE 限在 deleted_at IS NULL，所以这条 UPDATE 能落上）。
func (r *CollectionRepo) Update(ctx context.Context, c *collection.Collection) error {
	updates := map[string]any{
		"title":       c.Title(),
		"description": c.Description(),
		"cover":       c.Cover(),
		"is_display":  c.Displayed(),
		"sort_order":  c.SortOrder(),
		"updated_at":  c.UpdatedAt(),
	}
	if c.Deleted() {
		// 没有单独的删除时间字段：软删与最后一次修改是同一刻。
		updates["deleted_at"] = c.UpdatedAt()
	}
	res := r.db.WithContext(ctx).Model(&collectionRow{}).
		Where("id = ?", ctype.ID(c.ID())).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return collection.ErrNotFound
	}
	return nil
}

// Find 按 ID 读取合集（含未展示的）。
//
// 软删行由 GORM 自动排除；读不到一律返回 ErrNotFound（不区分"不存在"与
// "已删除"，两者对调用方是同一件事）。
func (r *CollectionRepo) Find(ctx context.Context, id collection.ID) (*collection.Collection, error) {
	var row collectionRow
	err := r.db.WithContext(ctx).Take(&row, "id = ?", ctype.ID(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toCollection(row), nil
}

// NextSortOrder 返回下一个可用序号（当前最大 + 1，空表为 1）。
func (r *CollectionRepo) NextSortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	err := r.db.WithContext(ctx).Model(&collectionRow{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// ListCards 列出展示中的合集卡片，按序号升序。
//
// 过滤"展示开关"在 SQL 里；篇数与封面回退来自目录树（outlineCardStats），
// 口径与读者详情一致：只数仍公开的文章、封面取 DFS 第一个公开文章节点。
//
// 分页为什么在 Go 里做：合集是**运营编排**的产物，数量以十计；而"开关关闭时
// 只保留仍有公开文章的合集"要先算出每张卡片的 article_count 才能判定，
// 在 SQL 里用别名过滤要包一层子查询，可读性换来的收益为零。
func (r *CollectionRepo) ListCards(
	ctx context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]collection.Card, int64, error) {
	rows, err := r.cardRows(ctx, onlyAdminAuthors)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(rows))
	return pageCards(rows, p), total, nil
}

// cardRows 查出全部展示中的合集卡片（篇数与封面回退来自目录树）。
func (r *CollectionRepo) cardRows(
	ctx context.Context, onlyAdminAuthors bool,
) ([]collection.Card, error) {
	type row struct {
		ID          ctype.ID
		Title       string
		Description string
		Cover       string
		CreatedAt   time.Time
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("collection_models AS c").
		Select("c.id, c.title, c.description, c.cover, c.created_at").
		Where("c.deleted_at IS NULL AND c.is_display = 1").
		Order("c.sort_order ASC, c.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 卡片读面以目录树为准：旧扁平收录中间表已冻结，这里完全不出现。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerCollection, onlyAdminAuthors)
	if err != nil {
		return nil, err
	}

	cards := make([]collection.Card, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		// 开关关闭时，一篇公开文章都凑不出的合集也不出现。
		if onlyAdminAuthors && s.ArticleCount == 0 {
			continue
		}
		cover := m.Cover
		if cover == "" {
			// 没上传封面时回退到 DFS 第一个公开文章节点的封面；它为空就是空。
			cover = s.Cover
		}
		cards = append(cards, collection.Card{
			ID:           collection.ID(m.ID),
			Title:        m.Title,
			Description:  m.Description,
			Cover:        cover,
			ArticleCount: s.ArticleCount,
			CreatedAt:    m.CreatedAt,
		})
	}
	return cards, nil
}

// ListAdminCards 列出全部合集（含未展示）。
//
// 篇数与回退封面与读者卡片同一口径：只数目录树里仍公开的文章节点、封面取
// DFS 第一个公开文章节点；失效节点不进数。
//
// 回退封面单独给出，Cover 仍保持库里的原值：编辑表单据此区分"我传过"与
// "这是回退来的"。
func (r *CollectionRepo) ListAdminCards(ctx context.Context) ([]collection.AdminCard, error) {
	type row struct {
		ID          ctype.ID
		Title       string
		Description string
		Cover       string
		IsDisplay   bool
		SortOrder   int
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("collection_models AS c").
		Select("c.id, c.title, c.description, c.cover, c.is_display, c.sort_order").
		Where("c.deleted_at IS NULL").
		Order("c.sort_order ASC, c.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 后台卡片不看「显示非管理员发布文章」这个读者开关，仍按"公开文章"口径。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerCollection, false)
	if err != nil {
		return nil, err
	}

	out := make([]collection.AdminCard, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		out = append(out, collection.AdminCard{
			ID:            collection.ID(m.ID),
			Title:         m.Title,
			Description:   m.Description,
			Cover:         m.Cover,
			Displayed:     m.IsDisplay,
			ArticleCount:  s.ArticleCount,
			SortOrder:     m.SortOrder,
			FallbackCover: s.Cover,
		})
	}
	return out, nil
}

// toCollection 把行结构转成聚合。
func toCollection(m collectionRow) *collection.Collection {
	return collection.Restore(collection.RestoreParams{
		ID:          collection.ID(m.ID),
		Title:       m.Title,
		Description: m.Description,
		Cover:       m.Cover,
		CreatorID:   domain.UserID(m.CreatorID),
		Displayed:   m.IsDisplay,
		SortOrder:   m.SortOrder,
		Deleted:     m.DeletedAt.Valid,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	})
}

// pageCards 按 Offset/Limit 截取。
func pageCards(cards []collection.Card, p domain.Page) []collection.Card {
	start := p.Offset
	if start > len(cards) {
		start = len(cards)
	}
	end := start + p.Limit
	if end > len(cards) {
		end = len(cards)
	}
	return cards[start:end]
}
