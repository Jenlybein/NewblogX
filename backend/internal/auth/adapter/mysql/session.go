// Package mysql 是认证模块的 MySQL 适配器。
//
// 只有这一层知道表名与列名（user_session_models）。所有对外错误都翻译成 apperr
// 错误码，上层看不到 gorm.ErrRecordNotFound 这种存储细节。
package mysql

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/domain"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// sessionRow 对应 user_session_models 表。
//
// 与 models.UserSessionModel 的差别：这里不带 UserModel 关联字段——认证只用
// user_id，关联预加载既浪费一次查询，也会让两个模块的表结构耦合。
type sessionRow struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	UserID           uint64
	RefreshTokenHash string
	IP               string
	Addr             string
	UA               string
	LastSeenAt       *time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

// TableName 固定表名，避免命名策略变化导致查错表。
func (sessionRow) TableName() string { return "user_session_models" }

// SessionRepo 实现 domain.SessionRepository。
type SessionRepo struct {
	db *gorm.DB
}

var _ domain.SessionRepository = (*SessionRepo)(nil)

// NewSessionRepo 构造会话仓储。
func NewSessionRepo(db *gorm.DB) *SessionRepo { return &SessionRepo{db: db} }

// Create 落库并回填 ID / CreatedAt。
func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	if s == nil {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if s.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		s.ID = domain.SessionID(next.Uint64())
	}

	now := time.Now()
	row := sessionRow{
		ID:               s.ID.Uint64(),
		CreatedAt:        now,
		UserID:           s.UserID,
		RefreshTokenHash: s.RefreshTokenHash,
		IP:               s.IP,
		Addr:             s.Addr,
		UA:               s.UA,
		LastSeenAt:       s.LastSeenAt,
		ExpiresAt:        s.ExpiresAt,
		RevokedAt:        s.RevokedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	s.CreatedAt = now
	return nil
}

// FindByRefreshHash 按刷新令牌哈希查会话。
//
// 这里只按哈希 + 未吊销过滤，过期由领域层 EnsureActive 判定：把"过期"交给
// 领域层，适配器就不需要接收 now 参数，测试也不用改系统时间。
func (r *SessionRepo) FindByRefreshHash(ctx context.Context, hash string) (domain.Session, error) {
	var row sessionRow
	err := r.db.WithContext(ctx).
		Where("refresh_token_hash = ? AND revoked_at IS NULL", hash).
		Take(&row).Error
	if err != nil {
		return domain.Session{}, translate(err)
	}
	return toSession(row), nil
}

// FindActive 按会话 ID + 用户 ID 查有效会话（校验归属，防止拿别人的会话 ID）。
func (r *SessionRepo) FindActive(ctx context.Context, id domain.SessionID, userID uint64) (domain.Session, error) {
	var row sessionRow
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", id.Uint64(), userID, time.Now()).
		Take(&row).Error
	if err != nil {
		return domain.Session{}, translate(err)
	}
	return toSession(row), nil
}

// Rotate 轮换刷新令牌：换新哈希、新过期时间，并更新客户端信息。
func (r *SessionRepo) Rotate(ctx context.Context, id domain.SessionID, newHash string, expiresAt time.Time, meta domain.Meta) error {
	return r.db.WithContext(ctx).
		Model(&sessionRow{}).
		Where("id = ?", id.Uint64()).
		Updates(map[string]any{
			"refresh_token_hash": newHash,
			"expires_at":         expiresAt,
			"last_seen_at":       time.Now(),
			"ip":                 meta.IP,
			"addr":               meta.Addr,
			"ua":                 meta.UA,
		}).Error
}

// Revoke 吊销单条会话（只写 revoked_at，保留行用于审计）。
func (r *SessionRepo) Revoke(ctx context.Context, id domain.SessionID, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&sessionRow{}).
		Where("id = ? AND revoked_at IS NULL", id.Uint64()).
		Update("revoked_at", at).Error
}

// RevokeAllForUser 吊销某用户的全部会话（改密码 / 踢下线）。
func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID uint64, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&sessionRow{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", at).Error
}

// Touch 更新最后活跃时间；失败由调用方决定是否忽略。
func (r *SessionRepo) Touch(ctx context.Context, id domain.SessionID, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&sessionRow{}).
		Where("id = ?", id.Uint64()).
		Update("last_seen_at", at).Error
}

// ListActive 列出用户当前所有有效会话。
func (r *SessionRepo) ListActive(ctx context.Context, userID uint64) ([]domain.Session, error) {
	var rows []sessionRow
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, time.Now()).
		Order("created_at desc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSession(row))
	}
	return out, nil
}

// translate 把存储层错误翻译成业务错误码。
func translate(err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperr.New(apperr.CodeAuthRefreshInvalid)
	}
	return err
}

// toSession 把 row 映射成领域实体。
func toSession(row sessionRow) domain.Session {
	return domain.Session{
		ID:               domain.SessionID(row.ID),
		UserID:           row.UserID,
		RefreshTokenHash: row.RefreshTokenHash,
		IP:               row.IP,
		Addr:             row.Addr,
		UA:               row.UA,
		CreatedAt:        row.CreatedAt,
		LastSeenAt:       row.LastSeenAt,
		ExpiresAt:        row.ExpiresAt,
		RevokedAt:        row.RevokedAt,
	}
}
