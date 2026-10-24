// Package domain 是认证与会话的领域层。
//
// 只管"会话什么时候有效、刷新要怎么轮换、什么情况必须拒绝"，
// 不碰数据库、不碰 redis、不碰 HTTP。
package domain

import (
	"time"

	"myblogx/internal/apperr"
)

// SessionID 是会话标识。
type SessionID uint64

// IsZero 判断 ID 是否为空。
func (id SessionID) IsZero() bool { return id == 0 }

// Uint64 返回底层值。
func (id SessionID) Uint64() uint64 { return uint64(id) }

// Session 是一次登录产生的服务端会话。
//
// 与"登录日志"的区别：这是当前登录态，可以被吊销；日志是历史，只追加。
// 一个 Refresh Token 对应一条 Session，用户每设备登录一次生成一条。
type Session struct {
	ID               SessionID
	UserID           uint64
	RefreshTokenHash string
	IP               string
	Addr             string
	UA               string
	CreatedAt        time.Time
	LastSeenAt       *time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

// IsActive 判断会话是否仍然有效（未吊销、未过期）。
//
// 三个条件缺一不可：只判过期会让"主动注销"失效，只判吊销会让过期会话永生。
func (s Session) IsActive(now time.Time) bool {
	if s.ID.IsZero() {
		return false
	}
	if s.RevokedAt != nil {
		return false
	}
	return now.Before(s.ExpiresAt)
}

// CanRefresh 判断能否用这条会话换新令牌。
//
// 与 IsActive 同一套条件，单独一个方法是为了让调用点读起来是"业务判断"
// 而不是"在 session 上做算术"。
func (s Session) CanRefresh(now time.Time) bool { return s.IsActive(now) }

// EnsureActive 校验会话有效，否则返回带错误码的失败。
//
// 返回 CodeAuthRefreshInvalid 而不是"已过期"：到底是被踢下线还是自然过期，
// 不该告诉客户端——两种情况客户端要做的事是一样的（重新登录）。
func (s Session) EnsureActive(now time.Time) error {
	if s.IsActive(now) {
		return nil
	}
	return apperr.New(apperr.CodeAuthRefreshInvalid)
}

// ExpiredBy 判断会话是否已经过期（不含吊销）。
//
// 清理任务用它挑可删的行：已吊销的会话要留一段时间用于审计，
// 只清自然过期的。
func (s Session) ExpiredBy(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// Meta 是登录时的客户端信息。
type Meta struct {
	IP   string
	Addr string
	UA   string
}

// Tokens 是一次登录/刷新签发的双令牌。
type Tokens struct {
	Access  string
	Refresh string
}

// Claims 是访问令牌里的载荷。
//
// 只放"鉴权当场要用"的字段：多放一个字段就多一份过期数据，
// 而且令牌体积会跟着涨。
type Claims struct {
	UserID       uint64
	SessionID    uint64
	TokenVersion uint32
	Username     string
	Role         int8
}

// Principal 是一次成功鉴权的结果。
//
// 中间件拿它往 gin.Context 里塞，业务层拿它判断"你是谁、什么角色"。
type Principal struct {
	UserID    uint64
	SessionID uint64
	Username  string
	Role      int8
}

// ValidateIdentifier 校验登录名。
func ValidateIdentifier(s string) error {
	if s == "" {
		return apperr.New(apperr.CodeUserInvalidCredential)
	}
	return nil
}

// ValidatePassword 校验密码长度。
//
// 只校验长度不做复杂度要求：复杂度规则会逼用户写 "Passw0rd!" 这类
// 好猜又好忘的组合，长度才是真正有效的约束。
func ValidatePassword(s string) error {
	if s == "" {
		return apperr.New(apperr.CodeUserPasswordRequired)
	}
	if len(s) < MinPasswordLen {
		return apperr.New(apperr.CodeUserPasswordTooShort)
	}
	return nil
}

// MinPasswordLen 是密码最小长度。
const MinPasswordLen = 8
