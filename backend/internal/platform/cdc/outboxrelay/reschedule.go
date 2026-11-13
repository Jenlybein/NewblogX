package outboxrelay

import (
	"context"
	"fmt"
	"time"

	"myblogx/internal/kernel/ctype"
)

// RescheduleError 让订阅者把**这一条事件本身**推迟到将来重投。
//
// 为什么需要它（而不是让订阅者直接改 available_at 再返回 nil）：
// Dispatch 在订阅者全部返回 nil 之后会立刻 markDone，把状态写成 done ——
// 订阅者自己写进去的 pending + 未来的 available_at 会被覆盖掉。所以"延后重投"
// 必须由中继在记账阶段完成：订阅者只表达"我现在不该被算作完成或失败"。
//
// 典型场景（ADR-0028）：AI 合规判定因作者当日额度用尽而无法执行，
// 这条事件要在额度恢复后（次日）再跑一次，既不是成功也不是失败。
//
// 它不消耗重试预算：claim 已经把 attempts +1，记账时改回领取前的值，
// 否则一条被推迟三天的事件会因为 attempts 涨到上限而变成死信。
type RescheduleError struct {
	// At 是这条事件重新可领取的时间。
	At time.Time
	// Reason 会写进 last_error，后台能看到"为什么这条被推迟"。
	Reason string
}

// Error 实现 error；文案只进日志与 last_error。
func (e *RescheduleError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("事件推迟到 %s 重投：%s", e.At.Format(time.RFC3339), e.Reason)
}

// markScheduled 把一条事件改回 pending 并设好下次可领取时间。
func (r *Relay) markScheduled(ctx context.Context, id ctype.ID, row outboxRow, re *RescheduleError) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&outboxRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       OutboxStatusPending,
			"available_at": re.At,
			"attempts":     row.Attempts,
			"last_error":   re.Reason,
			"updated_at":   now,
		}).Error
}
