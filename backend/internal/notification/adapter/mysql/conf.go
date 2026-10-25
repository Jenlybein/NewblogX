package mysql

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/domain"

	"gorm.io/gorm"
)

// confRow 对应 user_conf_models 表里通知开关相关的列。
//
// 该表还有收藏可见性、首页样式等字段，本模块只关心这四个开关。
type confRow struct {
	UserID                   uint64 `gorm:"primaryKey;autoIncrement:false"`
	DiggNoticeEnabled        bool
	CommentNoticeEnabled     bool
	FavorNoticeEnabled       bool
	PrivateChatNoticeEnabled bool
}

// TableName 固定表名。
func (confRow) TableName() string { return "user_conf_models" }

// ConfRepo 实现通知开关持久化端口。
type ConfRepo struct {
	db *gorm.DB
}

var _ domain.ConfRepository = (*ConfRepo)(nil)

// NewConf 构造开关仓储。
func NewConf(db *gorm.DB) *ConfRepo { return &ConfRepo{db: db} }

// Find 实现 domain.ConfRepository。
func (r *ConfRepo) Find(ctx context.Context, userID uint64) (domain.Conf, error) {
	var row confRow
	err := r.db.WithContext(ctx).
		Select("user_id", "digg_notice_enabled", "comment_notice_enabled",
			"favor_notice_enabled", "private_chat_notice_enabled").
		Where("user_id = ?", userID).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.Conf{}, apperr.New(apperr.CodeMessageConfMissing)
		}
		return domain.Conf{}, err
	}
	return domain.Conf{
		DiggNoticeEnabled:        row.DiggNoticeEnabled,
		CommentNoticeEnabled:     row.CommentNoticeEnabled,
		FavorNoticeEnabled:       row.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: row.PrivateChatNoticeEnabled,
	}, nil
}

// Replace 实现 domain.ConfRepository。
func (r *ConfRepo) Replace(ctx context.Context, userID uint64, conf domain.Conf) error {
	if _, err := r.Find(ctx, userID); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&confRow{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"digg_notice_enabled":         conf.DiggNoticeEnabled,
			"comment_notice_enabled":      conf.CommentNoticeEnabled,
			"favor_notice_enabled":        conf.FavorNoticeEnabled,
			"private_chat_notice_enabled": conf.PrivateChatNoticeEnabled,
		}).Error
}
