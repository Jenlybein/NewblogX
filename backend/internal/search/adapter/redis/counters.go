// Package redis 是搜索模块的 Redis 适配器（文章计数增量）。
package redis

import (
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/search/domain"
	"myblogx/models/ctype"
)

// Counters 实现 domain.CounterReader。
type Counters struct {
	deps redis_service.Deps
}

var _ domain.CounterReader = (*Counters)(nil)

// NewCounters 构造计数读取器。
func NewCounters(deps redis_service.Deps) *Counters { return &Counters{deps: deps} }

// Batch 返回文章计数增量。
func (c *Counters) Batch(ids []ctype.ID) domain.Counts {
	got := redis_article.GetBatchCounters(c.deps, ids)
	return domain.Counts{
		Favor:   got.FavorMap,
		Digg:    got.DiggMap,
		View:    got.ViewMap,
		Comment: got.CommentMap,
	}
}
