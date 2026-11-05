package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
)

// 本文件是「计数账本」的读侧唯一入口（#279）。
//
// 一个计数此刻对外可见的值 = 持久列（并账已入账的主值）+ 尚未并账的增量（Redis）。
// 这条加法过去散在每个读点上各写一遍：详情 / 列表 / 收藏内容 / 点赞写响应各拼一次，
// 首页「热门置顶」与作者置顶卡片则干脆只读列 —— 于是同一篇文章在同一时刻可以是
// 两个数（首页 5、点进去 7），而且只在那段"有增量还没并账"的窗口里出现，
// 恰是用户最容易看到的时间段。
//
// 现在口径只有这一处：
//   - 增量只从 CounterPort 来（Pending）；
//   - 权威值只由 Authoritative 定义（列 + 增量）；
//   - 任何读面都通过 ApplyTo / ApplyToItems / ApplyViews 写回，不许自己再拼加数。
//
// ADR-0025 把旧的「每个读点各自把列与增量相加」标为**被取代口径**。

// CountLedger 是文章计数的唯一权威读入口。
//
// 它只依赖 CounterPort（增量来源），持久列由调用方从刚读到的聚合同步给出：
// 账本负责"怎么合成权威值"，仓储负责"权威值的主值从哪来"。
type CountLedger struct {
	counters CounterPort
}

// NewCountLedger 构造账本读入口。
func NewCountLedger(counters CounterPort) CountLedger {
	return CountLedger{counters: counters}
}

// ledgerOf 从仓储集合取账本入口。
//
// app 层不再直接碰 CounterPort：所有增量读取都必须经过账本，
// 这样"第二套口径"没有机会再长出来。
func ledgerOf(r Repos) CountLedger {
	return NewCountLedger(r.Counters())
}

// Pending 返回一批文章尚未并账的增量。
//
// counters 为 nil 时返回空表（测试装配与无 Redis 环境的降级行为：
// 权威值退化为持久列本身）。
func (l CountLedger) Pending(ctx context.Context, ids []domain.ArticleID) (map[domain.ArticleID]domain.Counters, error) {
	out := make(map[domain.ArticleID]domain.Counters, len(ids))
	if len(ids) == 0 || l.counters == nil {
		return out, nil
	}
	deltas, err := l.counters.Load(ctx, ids)
	if err != nil {
		return nil, err
	}
	return deltas, nil
}

// Authoritative 返回权威计数：persisted（持久列）+ 待并账增量。
//
// persisted 由调用方从读到的快照填（`Counters()` / 卡片字段），本方法不查库：
// 账本只定义"权威值怎么算"，不定义"主值从哪读"。
func (l CountLedger) Authoritative(
	ctx context.Context,
	persisted map[domain.ArticleID]domain.Counters,
) (map[domain.ArticleID]domain.Counters, error) {
	if len(persisted) == 0 {
		return map[domain.ArticleID]domain.Counters{}, nil
	}
	ids := make([]domain.ArticleID, 0, len(persisted))
	for id := range persisted {
		ids = append(ids, id)
	}
	pending, err := l.Pending(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[domain.ArticleID]domain.Counters, len(persisted))
	for id, base := range persisted {
		delta := pending[id]
		out[id] = domain.Counters{
			View:    base.View + delta.View,
			Digg:    base.Digg + delta.Digg,
			Comment: base.Comment + delta.Comment,
			Favor:   base.Favor + delta.Favor,
		}
	}
	return out, nil
}

// ApplyTo 把权威计数写回文章聚合。
func (l CountLedger) ApplyTo(ctx context.Context, articles ...*domain.Article) error {
	if len(articles) == 0 {
		return nil
	}
	persisted := make(map[domain.ArticleID]domain.Counters, len(articles))
	for _, a := range articles {
		persisted[a.ID()] = a.Counters()
	}
	effective, err := l.Authoritative(ctx, persisted)
	if err != nil {
		return err
	}
	for _, a := range articles {
		a.SetCounters(effective[a.ID()])
	}
	return nil
}

// ApplyToItems 把权威计数写回收藏条目（收藏内容列表）。
func (l CountLedger) ApplyToItems(ctx context.Context, items []*favorite.Item) error {
	if len(items) == 0 {
		return nil
	}
	persisted := make(map[domain.ArticleID]domain.Counters, len(items))
	for _, it := range items {
		persisted[it.ArticleID()] = domain.Counters{
			View:    it.ViewCount(),
			Digg:    it.DiggCount(),
			Comment: it.CommentCount(),
			Favor:   it.FavorCount(),
		}
	}
	effective, err := l.Authoritative(ctx, persisted)
	if err != nil {
		return err
	}
	for _, it := range items {
		it.SetCounters(effective[it.ArticleID()])
	}
	return nil
}

// ApplyViews 返回一批文章的权威浏览量。
//
// 卡片（热门置顶 / 作者置顶）只暴露浏览量，所以入口形状比 Authoritative 窄一号；
// 但"列 + 增量"的加法仍然只发生在账本里。
func (l CountLedger) ApplyViews(ctx context.Context, persisted map[domain.ArticleID]int) (map[domain.ArticleID]int, error) {
	if len(persisted) == 0 {
		return map[domain.ArticleID]int{}, nil
	}
	base := make(map[domain.ArticleID]domain.Counters, len(persisted))
	for id, view := range persisted {
		base[id] = domain.Counters{View: view}
	}
	effective, err := l.Authoritative(ctx, base)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.ArticleID]int, len(effective))
	for id, c := range effective {
		out[id] = c.View
	}
	return out, nil
}
