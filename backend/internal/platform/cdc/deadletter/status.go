package deadletter

import (
	"errors"
	"time"

	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

// ErrSkipped 由重投执行器返回，表示"这条记录现在不需要重投"
// （业务侧已经补上了、或它已经不是待处理状态）。
//
// 放在 deadletter 而不是 dlq：执行器（river / outboxrelay）与编排层都要用它，
// 而它们共同的依赖只有这个包。这样不会出现"适配器反过来 import 编排层"的倒置。
// 文案用英文：archcheck R8 禁止 errors.New 里出现中文字面量（应走 apperr 错误码）。
// 这个哨兵不需要暴露给用户，只用于 errors.Is 判定。
var ErrSkipped = errors.New("deadletter: no replay needed")

// 死信的状态机。
//
//	pending ──人工重投成功──> resolved（终态，不再参与重投）
//	pending ──人工重投失败──> pending（retry_count+1，错误信息覆盖为本次失败）
//
// 只有两个状态是刻意的：
//   - 不设 `replaying` 中间态。重投是人工、低频、单实例操作，两个执行器都幂等
//     （ES 按文档 id index/delete；outbox 重置是纯状态写），重复执行无害。
//     中间态反而要额外写"僵尸 replaying 的恢复逻辑"，不划算。见 ADR-008。
//   - 不设 `gave_up`。放弃是人的判断，不是系统能决定的事；真要放弃就把记录留着，
//     它本来就不会再被自动处理。
//
// 注意：这两个值是**字符串列上的取值**，不是新列 —— 零 DDL 约束下没有结构变更。
const (
	// StatusPending 表示"同步失败留证，还没补上"。
	StatusPending = "pending"
	// StatusResolved 表示"已经人工重投成功"。
	StatusResolved = "resolved"
)

// ByIDs 按主键批量取死信记录。
//
// 顺序按 id 升序（稳定、可复现）；调用方如果要保持请求顺序，自己按 ID 索引。
func ByIDs(db *gorm.DB, ids []ctype.ID) ([]Record, error) {
	if db == nil || len(ids) == 0 {
		return nil, nil
	}
	list := make([]Record, 0, len(ids))
	if err := db.Where("id IN ?", ids).Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// MarkResolved 把一条死信标为"已补上"。
//
// 条件更新（`status <> resolved`）而不是无条件写：
// 重复提交同一批时，第二次不会把 resolved 的记录又"刷新"一遍 updated_at，
// 时间线因此保留第一次补上的时间点。返回是否真的更新了一行。
//
// **不动 error_code / error_msg**：那两个字段是"它当初为什么失败"的证据。
// 补上之后清空它们，等于把事故原因也一起抹掉了 —— 事后复盘时，
// 人要看的恰恰是"因为 X 失败了，后来在 T 时刻补上"。重投过程的时间线
// 在 replay_event_logs 里，不靠这两个字段。
func MarkResolved(db *gorm.DB, id ctype.ID, retryCount int) (bool, error) {
	if db == nil || id.IsZero() {
		return false, nil
	}
	res := db.Model(&Record{}).
		Where("id = ? AND status <> ?", id, StatusResolved).
		Updates(map[string]any{
			"status":      StatusResolved,
			"retry_count": retryCount,
			"updated_at":  time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// BumpRetry 记录一次失败的人工重投。
//
// 状态保持 pending（还能再点），retry_count 与错误信息覆盖为**本次**的失败原因 ——
// 死信的价值是"最后一次为什么没成功"，历史失败原因在回放日志里（ClickHouse 时间线），
// 不该跟这条记录抢同一个字段。
func BumpRetry(db *gorm.DB, id ctype.ID, retryCount int, errorCode, errorMsg string) error {
	if db == nil || id.IsZero() {
		return nil
	}
	return db.Model(&Record{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      StatusPending,
			"retry_count": retryCount,
			"error_code":  errorCode,
			"error_msg":   errorMsg,
			"updated_at":  time.Now(),
		}).Error
}
