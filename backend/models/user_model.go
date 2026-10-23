// 用户模型

package models

import (
	"errors"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 用户表
type UserModel struct {
	Model
	Username              string                  `gorm:"size:32;uniqueIndex:uk_user_username" json:"username"`
	Nickname              string                  `gorm:"size:32" json:"nickname"`
	Avatar                string                  `gorm:"size:256" json:"avatar"`
	Abstract              string                  `gorm:"size:256" json:"abstract"`
	RegisterSource        enum.RegisterSourceType `json:"register_source"` // 注册来源
	Password              string                  `gorm:"size:64" json:"-"`
	Email                 *string                 `gorm:"size:256;uniqueIndex:uk_user_email" json:"email"`
	OpenID                *string                 `gorm:"size:64;uniqueIndex:uk_user_open_id" json:"open_id"` // qq 登录的 openid
	Status                enum.UserStatus         `gorm:"default:1;index" json:"status"`
	TokenVersion          uint32                  `gorm:"default:1" json:"token_version"`
	LastPasswordChangedAt *time.Time              `json:"last_password_changed_at"`
	Role                  enum.RoleType           `gorm:"default:0" json:"role"`
	IP                    string                  `gorm:"size:64" json:"ip"`    // 注册时的 IP
	Addr                  string                  `gorm:"size:256" json:"addr"` // 注册时的地址
	UserConfModel         *UserConfModel          `gorm:"foreignKey:UserID;" json:"user_conf_model"`
	UserStatModel         *UserStatModel          `gorm:"foreignKey:UserID;" json:"user_stat_model"`
}

func (u *UserModel) BeforeCreate(tx *gorm.DB) (err error) {
	if u.Status == 0 {
		u.Status = enum.UserStatusActive
	}
	if u.TokenVersion == 0 {
		u.TokenVersion = 1
	}
	return u.Model.BeforeCreate(tx)
}

// AfterCreate 目前保留为兼容兜底。
// 业务主路径已改为显式调用 user_service.InitUserDefaults，
// 这里仅保证旧测试和遗留直写 Create(&UserModel{}) 不会缺失用户默认行。
func (u *UserModel) AfterCreate(tx *gorm.DB) error {
	confModel := UserConfModel{
		UserID:                   u.ID,
		FavoritesVisibility:      true,
		FollowVisibility:         true,
		FansVisibility:           true,
		HomeStyleID:              1,
		DiggNoticeEnabled:        true,
		CommentNoticeEnabled:     true,
		FavorNoticeEnabled:       true,
		PrivateChatNoticeEnabled: true,
		StrangerChatEnabled:      true,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&confModel).Error; err != nil {
		return err
	}

	statModel := UserStatModel{
		UserID:              u.ID,
		ViewCount:           0,
		FansCount:           0,
		FollowCount:         0,
		ArticleVisitedCount: 0,
		ArticleCount:        0,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&statModel).Error
}

// CodeAge 计算用户注册年龄（单位：年）
func (u *UserModel) CodeAge() int {
	return int(time.Since(u.CreatedAt).Hours() / 24 / 365)
}

func (u *UserModel) CheckTokenVersion(token uint32) bool {
	return u.TokenVersion == token
}

func (u *UserModel) CanLogin() bool {
	return u.Status.CanLogin()
}

func (u *UserModel) ValidateUserStatus() error {
	switch u.Status {
	case 0, 1:
		return nil
	case 2, 3:
		return errors.New(u.Status.String())
	default:
		return errors.New("用户状态异常")
	}
}

type UserConfModel struct {
	UserID                   ctype.ID   `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	UserModel                UserModel  `gorm:"foreignKey:UserID;references:ID" json:"-"`
	LikeTags                 []ctype.ID `gorm:"type:longtext;serializer:json" json:"like_tags"` // 用户偏好标签，关联公共文章标签 ID
	UpdatedUsernameDate      *time.Time `json:"updated_username_date"`                          // 上次修改用户名的时间
	FavoritesVisibility      bool       `json:"favorites_visibility"`                           // 收藏夹是否可见
	FollowVisibility         bool       `json:"followers_visibility"`                           // 关注是否可见
	FansVisibility           bool       `json:"fans_visibility"`                                // 粉丝是否可见
	HomeStyleID              ctype.ID   `json:"home_style_id"`                                  // 首页样式ID
	DiggNoticeEnabled        bool       `json:"digg_notice_enabled"`                            // 是否开启点赞通知
	CommentNoticeEnabled     bool       `json:"comment_notice_enabled"`                         // 是否开启评论通知
	FavorNoticeEnabled       bool       `json:"favor_notice_enabled"`                           // 是否开启收藏通知
	PrivateChatNoticeEnabled bool       `json:"private_chat_notice_enabled"`                    // 是否开启私聊通知
	StrangerChatEnabled      bool       `json:"stranger_msg_enabled"`                           // 是否开启陌生人私聊
}
