// Package redis 是用户模块的 Redis 适配器。
package redis

import (
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_user"
)

// UsernameAllocator 从 Redis 递增序列分配系统用户名。
type UsernameAllocator struct {
	deps redis_service.Deps
}

// NewUsernameAllocator 构造用户名分配器。
func NewUsernameAllocator(deps redis_service.Deps) *UsernameAllocator {
	return &UsernameAllocator{deps: deps}
}

// Next 返回下一个自动用户名。
func (a *UsernameAllocator) Next() (string, error) {
	return redis_user.NextAutoUsername(a.deps)
}
