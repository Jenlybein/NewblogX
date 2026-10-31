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
	// Scope 指定登录入口。空 = 读者站（行为与以前完全一致）；
	// "admin" = 管理后台，密码验证通过后还要校验角色，非管理员不签发令牌。
	//
	// 用 oneof 收窄而不是自由字符串：拼错（比如 "Admin"）会直接 400，
	// 而不是静默按读者站登录 —— 后者会让"后台登录拦不住非管理员"这个洞悄悄回来。
	Scope string `json:"scope" binding:"omitempty,oneof=admin"`
}

// SendEmailRequest 发送邮箱验证码。
type SendEmailRequest struct {
	// Type：1 注册、2 重置密码、3 绑定邮箱、4 邮箱登录。
	// 越界值按参数错误拒绝（oneof），不要让它一路走到发信层才变成"邮件发送失败"。
	Type  int8   `json:"type" binding:"required,oneof=1 2 3 4"`
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
