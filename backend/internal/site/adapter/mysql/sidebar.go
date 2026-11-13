package mysql

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
)

// sidebarCardRow 对应 sidebar_card_models 表。
//
// 这张表是右栏卡片的唯一配置源：种类、标题、排序、启用状态各占一列。
// 零列变更口径下，富文本正文放进同模块的 sidebar_card_body_models 表
// （新增表、零列变更，零 DDL 口径见 docs/architecture/overview.md §六 A1），不给这里加列。
type sidebarCardRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	Kind      string
	Title     string
	SortOrder int
	Enabled   bool
}

// TableName 固定表名。
func (sidebarCardRow) TableName() string { return "sidebar_card_models" }

// sidebarCardBodyRow 对应 sidebar_card_body_models 表：富文本卡的正文。
//
// 与卡片配置拆表的原因：正文是 longtext，而卡片表要保持"一行一张卡"的窄行；
// 配置整表替换时两张表在同一个事务里一起换，不存在正文挂在已删卡片上的窗口。
type sidebarCardBodyRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	CardID    ctype.ID `gorm:"index"`
	Body      string   `gorm:"type:longtext"`
}

// TableName 固定表名。
func (sidebarCardBodyRow) TableName() string { return "sidebar_card_body_models" }

// SidebarRepo 实现右栏卡片配置的持久化端口。
type SidebarRepo struct {
	db *gorm.DB
}

var _ domain.SidebarCardRepository = (*SidebarRepo)(nil)

// NewSidebar 构造右栏卡片仓储。
func NewSidebar(db *gorm.DB) *SidebarRepo { return &SidebarRepo{db: db} }

// ListEnabled 实现 domain.SidebarCardRepository。
func (r *SidebarRepo) ListEnabled(ctx context.Context) ([]domain.SidebarCard, error) {
	return r.list(ctx, true)
}

// ListAll 实现 domain.SidebarCardRepository。
func (r *SidebarRepo) ListAll(ctx context.Context) ([]domain.SidebarCard, error) {
	return r.list(ctx, false)
}

// list 按启用状态过滤并按运营顺序读取。
//
// 排序口径写死在仓储里：读者侧与管理侧必须看到同一顺序，
// 否则后台调完顺序刷新读者站还是旧的。
func (r *SidebarRepo) list(ctx context.Context, enabledOnly bool) ([]domain.SidebarCard, error) {
	var rows []sidebarCardRow
	query := r.db.WithContext(ctx)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	// 正文单独一遍查询（一次 IN 批量），不改变卡片表"一行一张卡"的窄行形状。
	bodies, err := r.bodiesByCard(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SidebarCard, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarCard{
			ID:        row.ID,
			Kind:      domain.SidebarCardKind(row.Kind),
			Title:     row.Title,
			SortOrder: row.SortOrder,
			Enabled:   row.Enabled,
			Content:   bodies[row.ID],
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// bodiesByCard 批量读取卡片正文，按卡片 ID 建索引。
func (r *SidebarRepo) bodiesByCard(ctx context.Context, rows []sidebarCardRow) (map[ctype.ID]string, error) {
	byCard := make(map[ctype.ID]string, len(rows))
	if len(rows) == 0 {
		return byCard, nil
	}
	ids := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var bodies []sidebarCardBodyRow
	if err := r.db.WithContext(ctx).Where("card_id IN ?", ids).Find(&bodies).Error; err != nil {
		return nil, err
	}
	for _, body := range bodies {
		byCard[body.CardID] = body.Body
	}
	return byCard, nil
}

// Replace 实现 domain.SidebarCardRepository。
//
// 全量替换放在一个事务里：删掉旧的、插入新的。中途失败时旧配置仍在，
// 读者站不会读到"空右栏"。
//
// 用 Unscoped 真删而不是软删：这张表的行是配置快照，软删只会让表随保存次数
// 线性增长，而且 ListAll 还得额外处理"软删的历史行"。
//
// 正文表与卡片表在同一个事务里一起清掉再一起写入：正文按 card_id 关联，
// 分开写会留下"卡片已换、正文还挂在旧 id 上"的窗口。
func (r *SidebarRepo) Replace(ctx context.Context, cards []domain.SidebarCard) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("1 = 1").Delete(&sidebarCardRow{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("1 = 1").Delete(&sidebarCardBodyRow{}).Error; err != nil {
			return err
		}
		if len(cards) == 0 {
			return nil
		}
		rows := make([]sidebarCardRow, 0, len(cards))
		bodies := make([]sidebarCardBodyRow, 0, len(cards))
		for _, card := range cards {
			id := card.ID
			if id.IsZero() {
				next, err := idgen.Next()
				if err != nil {
					return err
				}
				id = next
			}
			rows = append(rows, sidebarCardRow{
				ID:        id,
				CreatedAt: now,
				UpdatedAt: now,
				Kind:      string(card.Kind),
				Title:     card.Title,
				SortOrder: card.SortOrder,
				Enabled:   card.Enabled,
			})
			if card.Content == "" {
				continue
			}
			bodyID, err := idgen.Next()
			if err != nil {
				return err
			}
			bodies = append(bodies, sidebarCardBodyRow{
				ID:        bodyID,
				CreatedAt: now,
				UpdatedAt: now,
				CardID:    id,
				Body:      card.Content,
			})
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		if len(bodies) == 0 {
			return nil
		}
		return tx.Create(&bodies).Error
	})
}
