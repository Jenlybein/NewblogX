package deadletter

import (
	"gorm.io/gorm"
)

// DeleteByIDs 物理删除若干条死信，返回真正删掉的行数。
//
// 为什么是物理删除而不是软删：cdc_dead_letter 这张表**没有 deleted_at 列**
// （死信是"出问题的事件快照"，只做保留期清理），所以"删掉"只能是真删。
//
// 为什么可以安全删除（票 356 的调研结论）：
//   - 重投链路读的是这张表的行；删掉之后不会再有人捡起来重投，
//     但这正是用户要的"这条我没法处理，别留着了"；
//   - outbox 的 `dead` 行**不受影响**：relay 只扫描 pending / 到期的行，
//     永远不会回收 dead（见 outboxrelay/relay.go 的取行条件），
//     所以删死信不会让某条消息被"重新投递"或永久卡住；
//   - 回放日志（replay_event_logs）是独立表，删除死信不会连带删掉历史记录。
//
// 只按主键删除，不接受条件表达式：误删一整批的代价太大，调用方必须把 ID 列出来。
func DeleteByIDs(db *gorm.DB, ids []string) (int64, error) {
	if db == nil || len(ids) == 0 {
		return 0, nil
	}

	result := db.Where("id IN ?", ids).Delete(&Record{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
