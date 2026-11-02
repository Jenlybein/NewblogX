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
// 零列变更口径下，以后想加"参数"必须另建表或另建列（见 ADR-0018 的例外流程）。
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
	out := make([]domain.SidebarCard, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarCard{
			ID:        row.ID,
			Kind:      domain.SidebarCardKind(row.Kind),
			Title:     row.Title,
			SortOrder: row.SortOrder,
			Enabled:   row.Enabled,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// Replace 实现 domain.SidebarCardRepository。
//
// 全量替换放在一个事务里：删掉旧的、插入新的。中途失败时旧配置仍在，
// 读者站不会读到"空右栏"。
//
// 用 Unscoped 真删而不是软删：这张表的行是配置快照，软删只会让表随保存次数
// 线性增长，而且 ListAll 还得额外处理"软删的历史行"。
func (r *SidebarRepo) Replace(ctx context.Context, cards []domain.SidebarCard) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("1 = 1").Delete(&sidebarCardRow{}).Error; err != nil {
			return err
		}
		if len(cards) == 0 {
			return nil
		}
		rows := make([]sidebarCardRow, 0, len(cards))
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
		}
		return tx.Create(&rows).Error
	})
}
