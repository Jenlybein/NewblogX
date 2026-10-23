package models

import (
	"time"

	"myblogx/models/ctype"
)

// 一个 Refresh Token = 一条 Session 记录
// 用户每设备每登录一次 → 生成一条记录

// 用户会话表用于管理当前登录态，而不是登录历史日志。
type UserSessionModel struct {
	Model
	UserID           ctype.ID   `gorm:"index:idx_user_session_user_id;not null" json:"user_id"`
	UserModel        UserModel  `gorm:"foreignKey:UserID;references:ID" json:"-"`
	RefreshTokenHash string     `gorm:"size:64; uniqueIndex:uk_user_session_refresh_token" json:"-"` // 服务端记录refresh_token原文，哈希避免明文
	IP               string     `gorm:"size:64" json:"ip"`
	Addr             string     `gorm:"size:256" json:"addr"`
	UA               string     `gorm:"size:512" json:"ua"`
	LastSeenAt       *time.Time `json:"last_seen_at"` // 最近一次活跃时间
	ExpiresAt        time.Time  `gorm:"index:idx_user_session_expires_at" json:"expires_at"`
	RevokedAt        *time.Time `gorm:"index:idx_user_session_revoked_at" json:"revoked_at"` // 被“手动废弃”的时间（注销 / 踢下线）
}
