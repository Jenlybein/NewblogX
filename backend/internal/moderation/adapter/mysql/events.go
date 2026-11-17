package mysql

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/moderation/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// violationEventRow 对应 user_violation_events 表。
type violationEventRow struct {
	ID         ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt
	UserID     ctype.ID
	Level      string
	Score      int
	Source     string
	RefType    string
	RefID      ctype.ID
	Evidence   string
	Status     string
	RevokedBy  ctype.ID
	RevokedAt  *time.Time
	RevokeNote string
	IP         string
	Device     string
}

// TableName 固定表名。
func (violationEventRow) TableName() string { return "user_violation_events" }

// Repo 实现违规事件账本端口。
type Repo struct {
	db  *gorm.DB
	ids IDGenerator
}

// IDGenerator 生成事件 ID（复用全站的雪花 ID）。
type IDGenerator interface {
	NextID() (ctype.ID, error)
}

// New 构造仓储。
func New(db *gorm.DB, ids IDGenerator) *Repo { return &Repo{db: db, ids: ids} }

var _ domain.Repository = (*Repo)(nil)

// Append 追加一条事件。唯一键命中时返回 inserted=false。
//
// 实现要点：用 `ON DUPLICATE KEY` 语义（GORM 的 clause）而不是"先查再写" ——
// 两个请求同时判定同一张图违规时，先查再写会双双通过，同一件事被扣两次分。
func (r *Repo) Append(ctx context.Context, event domain.Event) (domain.Event, bool, error) {
	id, err := r.ids.NextID()
	if err != nil {
		return domain.Event{}, false, err
	}

	created := event.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	row := violationEventRow{
		ID:        id,
		CreatedAt: created,
		UpdatedAt: created,
		UserID:    event.UserID,
		Level:     string(event.Level),
		Score:     event.Score,
		Source:    string(event.Source),
		RefType:   event.RefType,
		RefID:     event.RefID,
		Evidence:  event.Evidence,
		Status:    string(domain.StatusActive),
		IP:        event.IP,
		Device:    event.Device,
	}

	// Ignore 在 MySQL 上编译成 INSERT IGNORE：唯一键冲突时静默跳过。
	// RowsAffected==0 就是"已经记过"，调用方据此跳过重复通知与重复处罚。
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return domain.Event{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.Event{}, false, nil
	}

	event.ID = row.ID
	event.CreatedAt = row.CreatedAt
	event.Status = domain.StatusActive
	event.Score = row.Score
	return event, true, nil
}

// ListByUser 按时间倒序取事件。
func (r *Repo) ListByUser(ctx context.Context, userID ctype.ID, limit int) ([]domain.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []violationEventRow
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEvent(row))
	}
	return out, nil
}

// FindByID 取单条事件。
func (r *Repo) FindByID(ctx context.Context, id ctype.ID) (domain.Event, bool, error) {
	var row violationEventRow
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.Event{}, false, nil
	}
	if err != nil {
		return domain.Event{}, false, err
	}
	return toEvent(row), true, nil
}

// Revoke 撤销一条事件：只改状态与撤销信息，不删行。
func (r *Repo) Revoke(ctx context.Context, id, operator ctype.ID, note string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&violationEventRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      string(domain.StatusRevoked),
			"revoked_by":  operator,
			"revoked_at":  at,
			"revoke_note": note,
			"updated_at":  at,
		}).Error
}

// toEvent 行 → 领域事件。
func toEvent(row violationEventRow) domain.Event {
	event := domain.Event{
		ID:        row.ID,
		UserID:    row.UserID,
		Level:     domain.Level(row.Level),
		Score:     row.Score,
		Source:    domain.Source(row.Source),
		RefType:   row.RefType,
		RefID:     row.RefID,
		Evidence:  row.Evidence,
		Status:    domain.Status(row.Status),
		IP:        row.IP,
		Device:    row.Device,
		CreatedAt: row.CreatedAt,
	}
	return event
}
