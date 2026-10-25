package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/app"
)

// outboxWriter 是 app.OutboxPort 的实现：把领域事件写进**事务内**的 outbox 表。
//
// 关键点：它持有的 *gorm.DB 是 WithinTx 传下来的事务句柄，
// 所以"业务写入"与"事件落库"在同一个事务里原子提交、一起回滚——
// 这正是事务型发件箱（transactional outbox）要保证的性质：
//
//	事件存在 ⇔ 业务已提交
//
// 这里**不做分发**：投递交给 CDC 中继（internal/platform/cdc/outboxrelay）
// 订阅 outbox 表的 binlog 异步完成；下游失败可重试、进程崩溃可恢复。
//
// 对比改造前（内存中同步调已注册 handler）：
//   - 旧：进程在"提交后、分发前"崩溃就会丢事件；
//   - 旧：下游失败只能让整个业务事务回滚，下游与业务强耦合；
//   - 新：两者都解决，代价是下游变成最终一致（而非同事务强一致）。
type outboxWriter struct {
	db *gorm.DB
}

// NewOutboxWriter 构造绑定到某个事务句柄的 outbox 写入器。
func NewOutboxWriter(db *gorm.DB) app.OutboxPort { return &outboxWriter{db: db} }

// Emit 把一条事件写进 outbox 表。
//
// db 为 nil 时静默丢弃（等价于"没有下游订阅"）：与改造前 outbox 为 nil 的
// 行为保持一致，避免装配缺依赖时反而打断业务写操作。
func (w *outboxWriter) Emit(ctx context.Context, e app.Event) error {
	if w == nil || w.db == nil {
		return nil
	}

	aggregateType := e.AggregateType
	if aggregateType == "" {
		aggregateType = "article"
	}

	// 时间戳显式写：GORM 会给零值 CreatedAt 自动填 time.Now()，
	// 但这里统一由本函数决定，便于测试固定时间与审计。
	now := time.Now()

	row := outboxRow{
		EventName:     e.Name,
		AggregateType: aggregateType,
		AggregateID:   e.AggregateID,
		Payload:       string(e.Payload),
		Status:        outboxStatusPending,
		AvailableAt:   now,
	}
	row.CreatedAt = now
	row.UpdatedAt = now

	return w.db.WithContext(ctx).Create(&row).Error
}
