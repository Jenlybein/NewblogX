package mysql

import (
	"context"

	"myblogx/internal/article/app"
)

// EventHandler 是领域事件的消费者。
//
// 由装配层注册（例如把 article.published 接到"重建 ES 投影""发系统通知"）。
// 放在这里而不是 app 里，是为了让 app 完全不认识下游模块。
type EventHandler func(ctx context.Context, e app.Event) error

// outbox 是 app.OutboxPort 的实现：按事件名同步分发给注册的处理器。
//
// 为什么当前是同步而不是真 outbox 表 + 异步消费：
// 改造前这些副作用（通知、评分、收藏快照同步）本来就是"要么同步、要么裸 goroutine"，
// 同步分发能保证行为等价、golden 可测；异步 outbox 属于 S4.6 的规划，
// 届时只需把这里换成"先落 outbox 表再投递"，app 与 handler 都不用动。
//
// 任一 handler 返回错误会让整个事务回滚——这正是我们要的：
// 通知发不出去就不该留下"文章已发布"的状态。
type outbox struct {
	handlers map[string][]EventHandler
}

// NewOutbox 构造事件分发器。
func NewOutbox() *outbox { return &outbox{handlers: make(map[string][]EventHandler)} }

// On 注册某事件名的处理器。
func (o *outbox) On(name string, h EventHandler) *outbox {
	o.handlers[name] = append(o.handlers[name], h)
	return o
}

// Emit 分发一条事件。
func (o *outbox) Emit(ctx context.Context, e app.Event) error {
	for _, h := range o.handlers[e.Name] {
		if h == nil {
			continue
		}
		if err := h(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
