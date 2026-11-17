package mysql

import (
	"context"
	"time"

	"myblogx/internal/moderation/domain"

	"gorm.io/gorm"
)

// appealRow 对应 user_violation_appeals 表。
type appealRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	EventID   uint64
	UserID    uint64
	Reason    string
	Status    string
	HandledBy uint64
	HandledAt *time.Time
	Note      string
}

// TableName 固定表名。
func (appealRow) TableName() string { return "user_violation_appeals" }

// AppealRepo 实现申诉端口。
type AppealRepo struct {
	db  *gorm.DB
	ids IDGenerator
}

// NewAppealRepo 构造申诉仓储。
func NewAppealRepo(db *gorm.DB, ids IDGenerator) *AppealRepo {
	return &AppealRepo{db: db, ids: ids}
}

var _ domain.AppealRepository = (*AppealRepo)(nil)

// Create 创建一条申诉。
func (r *AppealRepo) Create(ctx context.Context, appeal domain.Appeal) (domain.Appeal, error) {
	id, err := r.ids.NextID()
	if err != nil {
		return domain.Appeal{}, err
	}
	created := appeal.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	row := appealRow{
		ID:        id,
		CreatedAt: created,
		UpdatedAt: created,
		EventID:   appeal.EventID,
		UserID:    appeal.UserID,
		Reason:    appeal.Reason,
		Status:    string(domain.AppealPending),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.Appeal{}, err
	}
	appeal.ID = row.ID
	appeal.Status = domain.AppealPending
	appeal.CreatedAt = row.CreatedAt
	return appeal, nil
}

// FindByEvent 按事件取申诉。
func (r *AppealRepo) FindByEvent(ctx context.Context, eventID uint64) (domain.Appeal, bool, error) {
	var row appealRow
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.Appeal{}, false, nil
	}
	if err != nil {
		return domain.Appeal{}, false, err
	}
	return toAppeal(row), true, nil
}

// FindByID 按 ID 取申诉。
func (r *AppealRepo) FindByID(ctx context.Context, id uint64) (domain.Appeal, bool, error) {
	var row appealRow
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.Appeal{}, false, nil
	}
	if err != nil {
		return domain.Appeal{}, false, err
	}
	return toAppeal(row), true, nil
}

// List 按状态取申诉。
func (r *AppealRepo) List(ctx context.Context, status domain.AppealStatus, limit int) ([]domain.Appeal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	tx := r.db.WithContext(ctx).Model(&appealRow{})
	if status != "" {
		tx = tx.Where("status = ?", string(status))
	}
	var rows []appealRow
	if err := tx.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Appeal, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppeal(row))
	}
	return out, nil
}

// Handle 落一次处置结果。
func (r *AppealRepo) Handle(ctx context.Context, id, operator uint64, status domain.AppealStatus, note string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&appealRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     string(status),
			"handled_by": operator,
			"handled_at": at,
			"note":       note,
			"updated_at": at,
		}).Error
}

// toAppeal 行 → 领域申诉。
func toAppeal(row appealRow) domain.Appeal {
	return domain.Appeal{
		ID:        row.ID,
		EventID:   row.EventID,
		UserID:    row.UserID,
		Reason:    row.Reason,
		Status:    domain.AppealStatus(row.Status),
		HandledBy: row.HandledBy,
		HandledAt: row.HandledAt,
		Note:      row.Note,
		CreatedAt: row.CreatedAt,
	}
}
