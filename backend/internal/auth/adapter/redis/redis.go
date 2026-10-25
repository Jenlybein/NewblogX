// Package redis 是认证模块的 Redis 适配器：令牌黑名单与登录限流。
package redis

import (
	"context"
	"time"

	siteconf "myblogx/conf/site"
	"myblogx/internal/auth/domain"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_user"

	goredis "github.com/go-redis/redis/v8"
)

// blacklistPrefix 与既有 redis_jwt 保持同一前缀，新旧代码可以互相识别。
const blacklistPrefix = "token_blacklist_"

// Blacklist 实现 domain.TokenBlacklist。
type Blacklist struct {
	client *goredis.Client
}

var _ domain.TokenBlacklist = (*Blacklist)(nil)

// NewBlacklist 构造黑名单。
func NewBlacklist(client *goredis.Client) *Blacklist { return &Blacklist{client: client} }

// Add 把令牌写入黑名单，ttl 取令牌剩余有效期。
func (b *Blacklist) Add(ctx context.Context, token string, ttl time.Duration) error {
	if b.client == nil || token == "" {
		return nil
	}
	if ttl <= 0 {
		return nil
	}
	return b.client.Set(ctx, blacklistPrefix+token, "1", ttl).Err()
}

// Contains 判断令牌是否已被拉黑。
func (b *Blacklist) Contains(ctx context.Context, token string) (bool, error) {
	if b.client == nil || token == "" {
		return false, nil
	}
	_, err := b.client.Get(ctx, blacklistPrefix+token).Result()
	if err == goredis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// LoginGuard 实现 domain.LoginGuard，复用既有 redis_user 的计数实现。
//
// 限流阈值来自站点运行时配置，所以这里注一个取配置的函数而不是把配置拷贝进来：
// 管理员在后台改阈值后应立刻生效，而不是等重启。
type LoginGuard struct {
	deps redis_service.Options
	conf func() siteconf.Login
}

var _ domain.LoginGuard = (*LoginGuard)(nil)

// NewLoginGuard 构造登录限流器。
func NewLoginGuard(deps redis_service.Options, conf func() siteconf.Login) *LoginGuard {
	return &LoginGuard{deps: deps, conf: conf}
}

// Allowed 判断当前是否还允许尝试登录。
func (g *LoginGuard) Allowed(_ context.Context, account, ip string) (bool, error) {
	if g.deps.Client == nil {
		return true, nil
	}
	return redis_user.CheckLoginAllowed(g.deps, account, ip), nil
}

// RecordFailure 记一次失败。
func (g *LoginGuard) RecordFailure(_ context.Context, account, ip string) error {
	if g.deps.Client == nil {
		return nil
	}
	redis_user.RecordLoginFailure(g.deps, g.conf(), account, ip)
	return nil
}

// Reset 登录成功后清零计数与锁定。
func (g *LoginGuard) Reset(_ context.Context, account, ip string) error {
	if g.deps.Client == nil {
		return nil
	}
	redis_user.ResetLoginFailures(g.deps, account, ip)
	return nil
}
