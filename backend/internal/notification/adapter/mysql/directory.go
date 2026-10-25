package mysql

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/domain"

	"gorm.io/gorm"
)

// Users 实现通知模块的用户目录端口。
//
// 只读"注册时间 + 昵称头像"两项：公告可见性要靠注册时间，
// 消息快照要靠昵称头像。其它用户字段与通知无关。
type Users struct {
	db *gorm.DB
}

var _ domain.UserDirectory = (*Users)(nil)

// NewUsers 构造用户目录。
func NewUsers(db *gorm.DB) *Users { return &Users{db: db} }

// CreatedAt 返回注册时间。
func (u *Users) CreatedAt(ctx context.Context, userID uint64) (time.Time, error) {
	var row struct {
		CreatedAt time.Time
	}
	err := u.db.WithContext(ctx).
		Table("user_models").
		Select("created_at").
		Where("id = ? AND deleted_at IS NULL", userID).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, apperr.New(apperr.CodeNotifUserMissing)
		}
		return time.Time{}, err
	}
	return row.CreatedAt, nil
}

// Profile 返回昵称与头像快照。
func (u *Users) Profile(ctx context.Context, userID uint64) (string, string, error) {
	var row struct {
		Nickname string
		Avatar   string
	}
	err := u.db.WithContext(ctx).
		Table("user_models").
		Select("nickname", "avatar").
		Where("id = ? AND deleted_at IS NULL", userID).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", apperr.New(apperr.CodeNotifUserMissing)
		}
		return "", "", err
	}
	return row.Nickname, row.Avatar, nil
}

// ChatUnread 汇总私信未读。
//
// 只读一个聚合值，不碰 chat 模块的写路径。
type ChatUnread struct {
	db *gorm.DB
}

var _ domain.ChatUnreadCounter = (*ChatUnread)(nil)

// NewChatUnread 构造私信未读读取器。
func NewChatUnread(db *gorm.DB) *ChatUnread { return &ChatUnread{db: db} }

// UnreadTotal 实现通知模块的私信未读端口。
func (c *ChatUnread) UnreadTotal(ctx context.Context, userID uint64) (int64, error) {
	var total int64
	err := c.db.WithContext(ctx).
		Table("chat_session_models").
		Select("COALESCE(SUM(unread_count), 0)").
		Where("user_id = ? AND unread_count > 0 AND deleted_at IS NULL", userID).
		Scan(&total).Error
	return total, err
}
