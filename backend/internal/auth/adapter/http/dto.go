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
	// 图形验证码答案。站点启用 `site.login.captcha` 时必填，由
	// middleware.CaptchaMiddleware 在读本 handler 之前校验——它读的是同一份 JSON body
	// （io_util.ShouldBindJSONWithRecover 读完会把 body 复位，所以这里还能再绑一次）。
	// 开关关着时这两个字段可以不传，这里是空串，不影响校验。
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}

// SendEmailRequest 发送邮箱验证码。
type SendEmailRequest struct {
	// Type：1 注册、2 重置密码、3 绑定邮箱、4 邮箱登录。
	// 越界值按参数错误拒绝（oneof），不要让它一路走到发信层才变成"邮件发送失败"。
	Type  int8   `json:"type" binding:"required,oneof=1 2 3 4"`
	Email string `json:"email" binding:"required,email"`
	// 图形验证码答案，同 PwdLoginRequest：开关打开时由中间件先校验。
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
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

// SendSmsRequest 发送手机验证码。
type SendSmsRequest struct {
	// Type：1 登录/注册、3 设置密码前的复验。
	//
	// 取值与 platform/sms 的 Kind 一一对齐（2 曾是「绑定手机号」，随该功能下线并停用号位）；
	// 用 oneof 收窄，拼错直接 400，而不是一路走到发送层才变成"短信发送失败"。
	Type  int8   `json:"type" binding:"required,oneof=1 3"`
	Phone string `json:"phone" binding:"required"`
	// 图形验证码答案，同 PwdLoginRequest：开关打开时由中间件先校验。
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}

// PhoneLoginRequest 手机号 + 短信验证码登录（首次登录即注册）。
type PhoneLoginRequest struct {
	Phone string `json:"phone" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

// SetPasswordRequest 给尚无密码的账号设置首个密码。
//
// 带手机号与验证码是刻意的：只凭"已登录"就允许设密码，会让一个被盗的会话
// 把临时凭证升级成永久密码凭证。
type SetPasswordRequest struct {
	Phone       string `json:"phone" binding:"required"`
	Code        string `json:"code" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
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

// SessionItem 是登录设备列表的一行（已按设备聚合，一台设备一条）。
type SessionItem struct {
	ID ctype.ID `json:"id"`
	// Device 是 UA 解析出的人话描述（如 "Chrome 131 · Windows"），不再是 UA 原文。
	Device         string `json:"device"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	DeviceType     string `json:"device_type"`
	IP             string `json:"ip"`
	Addr           string `json:"addr"`
	// UA 保留原始串供排查，页面上不再直接展示。
	UA string `json:"ua"`
	// CreatedAt 是该设备的首次登录时间（保留旧字段名，兼容既有消费方）。
	CreatedAt time.Time `json:"created_at"`
	// FirstSeenAt 与 CreatedAt 同值，字段名更直白。
	FirstSeenAt time.Time  `json:"first_seen_at"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	// LoginCount 是该设备的累计登录次数（聚合进这一行的会话条数）。
	LoginCount int  `json:"login_count"`
	IsCurrent  bool `json:"is_current"`
}
