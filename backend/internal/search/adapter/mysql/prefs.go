// Package mysql 是搜索模块的 MySQL 适配器（用户偏好标签）。
package mysql

import (
	"context"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/search/domain"

	"gorm.io/gorm"
)

// confRow 对应 user_conf_models 表里搜索需要的列。
type confRow struct {
	UserID   ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	LikeTags []uint64 `gorm:"type:longtext;serializer:json"`
}

// TableName 固定表名。
func (confRow) TableName() string { return "user_conf_models" }

// Prefs 实现 domain.UserPreferences。
type Prefs struct {
	db *gorm.DB
}

var _ domain.UserPreferences = (*Prefs)(nil)

// NewPrefs 构造偏好读取器。
func NewPrefs(db *gorm.DB) *Prefs { return &Prefs{db: db} }

// LikeTagIDs 读用户偏好标签。
func (p *Prefs) LikeTagIDs(ctx context.Context, userID ctype.ID) ([]ctype.ID, error) {
	if p.db == nil || userID.IsZero() {
		return nil, nil
	}
	var row confRow
	if err := p.db.WithContext(ctx).Select("user_id", "like_tags").Take(&row, "user_id = ?", userID.Uint64()).Error; err != nil {
		return nil, err
	}
	out := make([]ctype.ID, 0, len(row.LikeTags))
	for _, id := range row.LikeTags {
		out = append(out, ctype.NewID(id))
	}
	return out, nil
}
