package domain

import (
	"context"
	"time"
)

// Clock 提供当前时间（注入，便于测过期与轮换）。
type Clock interface {
	Now() time.Time
}

// SessionRepository 是会话的持久化端口。
//
// 所有查询都要求传 now：过期判断必须由调用方给出的时间决定，
// 不能让仓储内部偷偷用 time.Now，否则"会话过期"就没法测。
type SessionRepository interface {
	// Create 落库并回填 ID。
	Create(ctx context.Context, s *Session) error
	// FindByRefreshHash 按刷新令牌哈希查会话；不存在时返回 CodeAuthRefreshInvalid。
	FindByRefreshHash(ctx context.Context, hash string) (Session, error)
	// FindActive 按会话 ID + 用户 ID 查有效会话（会校验归属，防止拿别人的会话 ID）。
	FindActive(ctx context.Context, id SessionID, userID uint64) (Session, error)
	// Rotate 轮换：换新哈希、新过期时间、更新客户端信息。
	Rotate(ctx context.Context, id SessionID, newHash string, expiresAt time.Time, meta Meta) error
	// Revoke 吊销单条会话。
	Revoke(ctx context.Context, id SessionID, at time.Time) error
	// RevokeAllForUser 吊销某用户的全部会话（改密码/踢下线用）。
	RevokeAllForUser(ctx context.Context, userID uint64, at time.Time) error
	// Touch 更新最后活跃时间。
	Touch(ctx context.Context, id SessionID, at time.Time) error
	// ListActive 列出用户当前所有有效会话（"登录设备管理"页用）。
	ListActive(ctx context.Context, userID uint64) ([]Session, error)
}

// TokenIssuer 负责访问令牌的签发与解析。
type TokenIssuer interface {
	// Issue 签发访问令牌。
	Issue(claims Claims) (string, error)
	// Parse 解析并校验签名与有效期，返回载荷。
	Parse(token string) (Claims, error)
}

// SecretGenerator 生成不可预测的随机串（刷新令牌）。
type SecretGenerator interface {
	// NewRefreshToken 返回一个新的刷新令牌原文。
	NewRefreshToken() (string, error)
}

// Hasher 计算刷新令牌的存储哈希。
//
// 存哈希不存原文：数据库泄露时，攻击者拿到的是哈希，不能直接拿去换令牌。
type Hasher interface {
	SHA256(s string) string
}

// TokenBlacklist 是访问令牌的黑名单。
//
// 为什么无状态 JWT 还需要黑名单：注销要让"手里的旧 access token"立刻失效，
// 但 access token 是自包含的、改不了。只能把它的 id 记下来，鉴权时查一次。
type TokenBlacklist interface {
	// Add 把令牌加入黑名单，ttl 取令牌剩余有效期（过期后自动清除，不用永久存）。
	Add(ctx context.Context, token string, ttl time.Duration) error
	// Contains 判断令牌是否在黑名单里。
	Contains(ctx context.Context, token string) (bool, error)
}

// LoginGuard 是登录失败次数限制。
//
// 按「账号 + IP」双维度计数：只按账号会被恶意锁别人的号，
// 只按 IP 会被换 IP 绕过。
type LoginGuard interface {
	// Allowed 判断当前是否还允许尝试登录。
	Allowed(ctx context.Context, account, ip string) (bool, error)
	// RecordFailure 记一次失败。
	RecordFailure(ctx context.Context, account, ip string) error
	// Reset 登录成功后清零。
	Reset(ctx context.Context, account, ip string) error
}
