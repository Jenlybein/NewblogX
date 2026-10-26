package http

import (
	"myblogx/internal/kernel/ctype"
	"time"
)

// 本文件是认证 HTTP 层的请求/响应结构。
// 字段名与旧接口保持一致，前端不需要改。

// PwdLoginRequest 用户名/邮箱 + 密码登录。
type PwdLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// SendEmailRequest 发送邮箱验证码。
type SendEmailRequest struct {
	// Type：1 注册、2 重置密码、3 绑定邮箱、4 邮箱登录。
	Type  int8   `json:"type"`
	Email string `json:"email" binding:"required,email"`
}

// SendEmailResponse 返回验证码记录 ID。
type SendEmailResponse struct {
	ID string `json:"id"`
}

// RegisterEmailRequest 邮箱注册（邮箱来自验证码中间件）。
type RegisterEmailRequest struct {
	Pwd string `json:"pwd" binding:"required"`
}

// QQLoginRequest QQ 授权码。
type QQLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

// ResetPasswordRequest 邮箱验证码重置密码。
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdatePasswordRequest 已登录用户改密码。
type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// SessionItem 是登录设备列表的一行。
type SessionItem struct {
	ID         ctype.ID   `json:"id"`
	IP         string     `json:"ip"`
	Addr       string     `json:"addr"`
	UA         string     `json:"ua"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	IsCurrent  bool       `json:"is_current"`
}
