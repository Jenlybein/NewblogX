package apptest

import (
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
)

// 本文件给"端口级 fake"补上事务回滚支持。
//
// 为什么要连 outbox 和 counters 一起回滚：生产里它们分别是同一事务的
// outbox 表与 Redis 写入，事务失败都不该留下痕迹。只回滚仓储会让
// "回滚后无副作用"的断言变成假绿。

// Snapshot 保存作者统计状态。
func (s *AuthorStats) Snapshot() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[domain.UserID][2]int, len(s.deltas))
	for k, v := range s.deltas {
		cp[k] = v
	}
	return cp
}

// RestoreState 恢复作者统计状态。
func (s *AuthorStats) RestoreState(state any) {
	cp, ok := state.(map[domain.UserID][2]int)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deltas = cp
}

// Snapshot 保存已发事件。
func (o *Outbox) Snapshot() any {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]app.Event(nil), o.events...)
}

// RestoreState 恢复已发事件。
func (o *Outbox) RestoreState(state any) {
	evts, ok := state.([]app.Event)
	if !ok {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = evts
}

// Snapshot 保存计数增量。
func (c *Counters) Snapshot() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[domain.ArticleID]domain.Counters, len(c.byID))
	for k, v := range c.byID {
		cp[k] = v
	}
	return cp
}

// RestoreState 恢复计数增量。
func (c *Counters) RestoreState(state any) {
	cp, ok := state.(map[domain.ArticleID]domain.Counters)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID = cp
}

// Snapshot 保存评论数据。
func (c *Comments) Snapshot() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[domain.ArticleID][]uint64, len(c.byArticle))
	for k, v := range c.byArticle {
		cp[k] = append([]uint64(nil), v...)
	}
	return cp
}

// RestoreState 恢复评论数据。
func (c *Comments) RestoreState(state any) {
	cp, ok := state.(map[domain.ArticleID][]uint64)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byArticle = cp
}

// Snapshot 保存点赞关系。
//
// 活行与软删行都要存：只存活行的话，事务回滚会把"软删残行"留在库里，
// 于是"回滚后无副作用"的断言变成假绿。
func (d *Diggs) Snapshot() any {
	d.mu.Lock()
	defer d.mu.Unlock()
	cpLive := make(map[itemKey2]struct{}, len(d.live))
	for k := range d.live {
		cpLive[k] = struct{}{}
	}
	cpSoft := make(map[itemKey2]struct{}, len(d.soft))
	for k := range d.soft {
		cpSoft[k] = struct{}{}
	}
	return diggState{live: cpLive, soft: cpSoft, events: append([]string(nil), d.events...)}
}

// RestoreState 恢复点赞关系。
func (d *Diggs) RestoreState(state any) {
	s, ok := state.(diggState)
	if !ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.live = s.live
	d.soft = s.soft
	d.events = s.events
}

// diggState 是点赞 fake 的快照。
type diggState struct {
	live   map[itemKey2]struct{}
	soft   map[itemKey2]struct{}
	events []string
}
