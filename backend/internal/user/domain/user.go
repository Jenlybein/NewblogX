// Package domain 是用户模块的领域层。
//
// 这里只有"用户是什么、谁能对用户做什么"，不碰数据库、不碰 HTTP。
// 依赖注入一律走端口（见 port.go），时间走 Clock，ID 是独立类型。
package domain

import (
	"strconv"
	"strings"
	"time"
)

// UserID 是用户标识。
//
// 独立类型而不是裸 uint64：把 UserID 传到需要 ArticleID 的位置会编译期报错。
// 与 models/ctype.ID 的转换只在 adapter 层做。
type UserID uint64

// IsZero 判断 ID 是否为空。
func (id UserID) IsZero() bool { return id == 0 }

// Uint64 返回底层值，供 adapter 写库使用。
func (id UserID) Uint64() uint64 { return uint64(id) }

// String 便于日志与错误信息输出。
func (id UserID) String() string { return strconv.FormatUint(uint64(id), 10) }

// Status 是账号状态。
type Status int8

const (
	// StatusActive 正常，可登录。
	StatusActive Status = 1
	// StatusDisabled 被管理员禁用，可被解禁。
	StatusDisabled Status = 2
	// StatusBanned 被封禁，不可自助恢复。
	StatusBanned Status = 3
)

// CanLogin 只有正常状态允许登录。
func (s Status) CanLogin() bool { return s == StatusActive }

// IsUsable 判断账号是否处于可写状态（发帖、评论等）。
func (s Status) IsUsable() bool { return s == StatusActive }

// String 返回状态的中文说明。
func (s Status) String() string {
	switch s {
	case StatusActive:
		return "正常"
	case StatusDisabled:
		return "账号已被禁用"
	case StatusBanned:
		return "账号已被封禁"
	default:
		return "未知状态"
	}
}

// Role 是账号角色。
type Role int8

const (
	// RoleAdmin 管理员。
	RoleAdmin Role = 1
	// RoleUser 注册用户。
	RoleUser Role = 2
	// RoleGuest 访客。
	RoleGuest Role = 3
)

// IsAdmin 判断是否管理员。
func (r Role) IsAdmin() bool { return r == RoleAdmin }

// String 返回角色的中文说明。
func (r Role) String() string {
	switch r {
	case RoleAdmin:
		return "管理员"
	case RoleUser:
		return "普通用户"
	case RoleGuest:
		return "访客"
	default:
		return "未知角色"
	}
}

// RegisterSource 是注册来源。
type RegisterSource int8

const (
	// SourceEmail 邮箱注册。
	SourceEmail RegisterSource = 1
	// SourceQQ QQ 登录首次建档。
	SourceQQ RegisterSource = 2
	// SourceTerminal 终端注册。
	SourceTerminal RegisterSource = 3
	// SourceAdmin 管理员创建。
	SourceAdmin RegisterSource = 4
)

// User 是用户聚合根。
//
// 只放"跨用例都要用"的字段；偏好设置（可见性、通知开关）属于 ProfileSettings，
// 单独一个值对象，避免聚合根膨胀成一张表的镜像。
type User struct {
	ID             UserID
	CreatedAt      time.Time
	Username       string
	Nickname       string
	Avatar         string
	Abstract       string
	Email          *string
	PasswordHash   string
	OpenID         *string
	Status         Status
	Role           Role
	RegisterSource RegisterSource
	TokenVersion   uint32
	IP             string
	Addr           string
}

// CodeAge 返回注册年数（用于展示"加入 N 年"）。
func (u User) CodeAge(now time.Time) int {
	if u.CreatedAt.IsZero() {
		return 0
	}
	return int(now.Sub(u.CreatedAt).Hours() / 24 / 365)
}

// HasPassword 判断账号是否设置过密码。
//
// 邮箱/QQ 登录建档的账号可能没密码，前端要据此隐藏"修改密码"入口。
func (u User) HasPassword() bool { return u.PasswordHash != "" }

// TokenVersionMatches 判断令牌携带的版本号是否与当前一致。
//
// 改密码/踢下线时把版本号 +1，所有旧令牌立即失效——这是无状态 JWT 唯一的
// 主动失效手段。
func (u User) TokenVersionMatches(v uint32) bool { return u.TokenVersion == v }

// IsZero 判断是否为空用户（仓储查不到时的零值）。
func (u User) IsZero() bool { return u.ID.IsZero() }

// Settings 是用户偏好设置（对应 user_conf 表）。
type Settings struct {
	UserID                   UserID
	LikeTagIDs               []uint64
	UpdatedUsernameAt        *time.Time
	FavoritesVisibility      bool
	FollowVisibility         bool
	FansVisibility           bool
	HomeStyleID              uint64
	DiggNoticeEnabled        bool
	CommentNoticeEnabled     bool
	FavorNoticeEnabled       bool
	PrivateChatNoticeEnabled bool
	StrangerChatEnabled      bool
}

// DefaultSettings 返回新用户的默认偏好。
//
// 默认值写在领域层而不是数据库：这里能写测试，数据库默认值不能。
func DefaultSettings(userID UserID) Settings {
	return Settings{
		UserID:                   userID,
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
}

// Stats 是用户主页的冗余统计（对应 user_stat 表）。
type Stats struct {
	UserID              UserID
	ViewCount           int
	FansCount           int
	FollowCount         int
	ArticleVisitedCount int
	ArticleCount        int
}

// ZeroStats 返回新用户的空统计。
func ZeroStats(userID UserID) Stats {
	return Stats{UserID: userID}
}

// Display 是其他模块需要的用户展示快照。
//
// 跨模块读用户信息一律返回这个结构（见 app.UserInfoPort），不要返回 User——
// 那样会把密码哈希、邮箱之类的敏感字段带出边界。
type Display struct {
	ID       UserID
	Nickname string
	Avatar   string
	Abstract string
	Role     Role
}

// normalizeNickname 去掉首尾空白。
//
// 不做 trim 的话" 张三"和"张三"会显示成两个人，还都能存进去。
func normalizeNickname(s string) string { return strings.TrimSpace(s) }
