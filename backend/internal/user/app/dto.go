package app

import (
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// LikeTagItem 是"偏好标签"的展示项（ID + 标题）。
type LikeTagItem struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

// DetailView 是当前登录用户的详细资料。
//
// 字段名与既有前端契约逐字一致——改一个名字前端就白屏，所以这里不做"美化"。
type DetailView struct {
	ID                       uint64        `json:"id"`
	CreatedAt                time.Time     `json:"created_at"`
	Username                 string        `json:"username"`
	Nickname                 string        `json:"nickname"`
	AvatarImageID            *uint64       `json:"avatar_image_id,omitempty"`
	Avatar                   string        `json:"avatar"`
	Abstract                 string        `json:"abstract"`
	Email                    *string       `json:"email"`
	HasPassword              bool          `json:"has_password"`
	RegisterSource           int8          `json:"register_source"`
	CodeAge                  int           `json:"code_age"`
	LikeTagIDs               []uint64      `json:"like_tag_ids"`
	LikeTagItems             []LikeTagItem `json:"like_tag_items"`
	UpdatedUsernameDate      *time.Time    `json:"updated_username_date"`
	FavoritesVisibility      bool          `json:"favorites_visibility"`
	FollowVisibility         bool          `json:"followers_visibility"`
	FansVisibility           bool          `json:"fans_visibility"`
	HomeStyleID              uint64        `json:"home_style_id"`
	DiggNoticeEnabled        bool          `json:"digg_notice_enabled"`
	CommentNoticeEnabled     bool          `json:"comment_notice_enabled"`
	FavorNoticeEnabled       bool          `json:"favor_notice_enabled"`
	PrivateChatNoticeEnabled bool          `json:"private_chat_notice_enabled"`
	StrangerChatEnabled      bool          `json:"stranger_msg_enabled"`
}

// SettingsPatch 是偏好设置的局部更新。
//
// 全是指针：false 是合法值（关闭通知），用 bool 判断"没传"和"关掉"就分不清了。
type SettingsPatch struct {
	LikeTagIDs               *[]uint64
	FavoritesVisibility      *bool
	FollowVisibility         *bool
	FansVisibility           *bool
	HomeStyleID              *uint64
	DiggNoticeEnabled        *bool
	CommentNoticeEnabled     *bool
	FavorNoticeEnabled       *bool
	PrivateChatNoticeEnabled *bool
	StrangerChatEnabled      *bool
}

// IsEmpty 判断是否什么都没改。
func (p SettingsPatch) IsEmpty() bool {
	return p.LikeTagIDs == nil &&
		p.FavoritesVisibility == nil && p.FollowVisibility == nil && p.FansVisibility == nil &&
		p.HomeStyleID == nil && p.DiggNoticeEnabled == nil && p.CommentNoticeEnabled == nil &&
		p.FavorNoticeEnabled == nil && p.PrivateChatNoticeEnabled == nil && p.StrangerChatEnabled == nil
}

// Apply 把补丁作用到现有设置上。
func (p SettingsPatch) Apply(s domain.Settings) domain.Settings {
	if p.LikeTagIDs != nil {
		s.LikeTagIDs = *p.LikeTagIDs
	}
	if p.FavoritesVisibility != nil {
		s.FavoritesVisibility = *p.FavoritesVisibility
	}
	if p.FollowVisibility != nil {
		s.FollowVisibility = *p.FollowVisibility
	}
	if p.FansVisibility != nil {
		s.FansVisibility = *p.FansVisibility
	}
	if p.HomeStyleID != nil {
		s.HomeStyleID = *p.HomeStyleID
	}
	if p.DiggNoticeEnabled != nil {
		s.DiggNoticeEnabled = *p.DiggNoticeEnabled
	}
	if p.CommentNoticeEnabled != nil {
		s.CommentNoticeEnabled = *p.CommentNoticeEnabled
	}
	if p.FavorNoticeEnabled != nil {
		s.FavorNoticeEnabled = *p.FavorNoticeEnabled
	}
	if p.PrivateChatNoticeEnabled != nil {
		s.PrivateChatNoticeEnabled = *p.PrivateChatNoticeEnabled
	}
	if p.StrangerChatEnabled != nil {
		s.StrangerChatEnabled = *p.StrangerChatEnabled
	}
	return s
}

// CreateInput 是管理员创建用户时的输入。
type CreateInput struct {
	Username string
	Nickname string
	Password string
	Email    *string
	Role     domain.Role
	Status   domain.Status
	IP       string
	Addr     string
}

// UpdateResult 是一次资料更新的结果。
type UpdateResult struct {
	User           domain.User
	Changed        []string
	SnapshotSynced bool
}

// notFound 是"用户不存在"的统一返回。
func notFound() error { return apperr.New(apperr.CodeUserNotFound) }
