package mysql

import (
	"context"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/models/ctype"
)

// counterPort 是 app.CounterPort 的实现：计数增量落在 Redis。
//
// 为什么计数不写文章表：点赞/浏览是高频写，直接更 MySQL 会成为热点行。
// 既有设计就是"Redis 累加 + 定时落库"，这里保持一致。
type counterPort struct {
	deps redis_service.Deps
}

// NewCounterPort 构造计数端口。
//
// 传入的 deps.Client 为 nil 时退化为 no-op：本地单测/无 Redis 环境下
// 文章功能仍可用，只是计数不增长（与改造前的降级行为一致）。
func NewCounterPort(deps redis_service.Deps) app.CounterPort {
	return &counterPort{deps: deps}
}

// Add 累加计数增量。
func (c *counterPort) Add(ctx context.Context, id domain.ArticleID, delta domain.Counters) error {
	_ = ctx
	if c.deps.Client == nil {
		return nil
	}
	aid := ctype.ID(id.Uint64())
	if delta.View != 0 {
		if err := redis_article.SetCacheView(c.deps, aid, delta.View); err != nil {
			return err
		}
	}
	if delta.Digg != 0 {
		if err := redis_article.SetCacheDigg(c.deps, aid, delta.Digg); err != nil {
			return err
		}
	}
	if delta.Favor != 0 {
		if err := redis_article.SetCacheFavorite(c.deps, aid, delta.Favor); err != nil {
			return err
		}
	}
	if delta.Comment != 0 {
		if err := redis_article.SetCacheComment(c.deps, aid, delta.Comment); err != nil {
			return err
		}
	}
	return nil
}

// Load 批量读取计数增量。
func (c *counterPort) Load(ctx context.Context, ids []domain.ArticleID) (map[domain.ArticleID]domain.Counters, error) {
	_ = ctx
	out := make(map[domain.ArticleID]domain.Counters, len(ids))
	if c.deps.Client == nil || len(ids) == 0 {
		return out, nil
	}

	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, ctype.ID(id.Uint64()))
	}
	batch := redis_article.GetBatchCounters(c.deps, raw)
	for _, id := range ids {
		key := ctype.ID(id.Uint64())
		out[id] = domain.Counters{
			View:    batch.ViewMap[key],
			Digg:    batch.DiggMap[key],
			Favor:   batch.FavorMap[key],
			Comment: batch.CommentMap[key],
		}
	}
	return out, nil
}
