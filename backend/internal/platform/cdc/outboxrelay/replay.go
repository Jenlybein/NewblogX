package outboxrelay

// outbox_relay 死信的重投执行器
//
// 这条链路的"重投"不是立刻投递，而是**把发件箱行退回可领取状态**：
//
//	dead ──人工重投──> pending（attempts 归零，available_at 立即）
//
// 为什么不直接调用 dispatcher 投一次：中继已经有自己的重试、退避、租约与并发控制。
// 绕过它另开一条投递路径，等于把这些保证复制一遍，还多出"两条路径同时投同一条事件"
// 的风险。退回状态让中继按它自己的节奏重投，是这里唯一稳妥的做法。
// 代价是重投不是即时的：最快也要等中继下一轮扫描（默认退避 2s 起步）。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"

	"gorm.io/gorm"
)

// ReplayStream 是发件箱中继写进死信表的 stream 值。
const ReplayStream = "outbox_relay"

// outboxJobIDPrefix 与 saveDeadLetter 里的 CdcJobID 格式一一对应。
const outboxJobIDPrefix = "outbox:"

// Replayer 把 outbox_relay 的死信退回发件箱，交给中继重投。
type Replayer struct {
	db *gorm.DB
}

// NewReplayer 构造执行器。
func NewReplayer(db *gorm.DB) *Replayer {
	return &Replayer{db: db}
}

// Streams 实现 dlq.Replayer。
func (r *Replayer) Streams() []string { return []string{ReplayStream} }

// Replay 实现 dlq.Replayer。
func (r *Replayer) Replay(ctx context.Context, record deadletter.Record) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("outboxrelay: 数据库未初始化，无法重投")
	}
	id, err := parseOutboxJobID(record.CdcJobID)
	if err != nil {
		return err
	}

	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&outboxRow{}).
		// 条件里带 status=dead：如果这条事件在中继那边已经失败了更多次、
		// 或者已经被别的操作改回 pending，这里就什么都不做（幂等 + 不抢别人的操作）。
		Where("id = ? AND status = ?", id, OutboxStatusDead).
		Updates(map[string]any{
			"status":       OutboxStatusPending,
			"attempts":     0,
			"available_at": now,
			"last_error":   "",
			"updated_at":   now,
		})
	if res.Error != nil {
		return fmt.Errorf("outboxrelay: 重置发件箱事件失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// 不是 dead 状态：要么已被人工补上变成 done，要么中继正在处理。
		// 这两种情况都不需要我们再做什么，交给编排层记成 skipped。
		return deadletter.ErrSkipped
	}
	return nil
}

// parseOutboxJobID 从 "outbox:<id>" 里取出主键。
func parseOutboxJobID(jobID string) (ctype.ID, error) {
	raw := strings.TrimSpace(jobID)
	if !strings.HasPrefix(raw, outboxJobIDPrefix) {
		return "", fmt.Errorf("outboxrelay: 无法从 cdc_job_id 解析事件主键（期望 outbox:<id>，实际 %q）", jobID)
	}
	id, err := ctype.ParseID(strings.TrimPrefix(raw, outboxJobIDPrefix))
	if err != nil || id.IsZero() {
		return "", fmt.Errorf("outboxrelay: cdc_job_id 里的主键不合法: %q", jobID)
	}
	return id, nil
}
