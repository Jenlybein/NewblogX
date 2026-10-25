package river

import (
	"strings"

	"myblogx/internal/platform/cdc/outboxrelay"

	"github.com/go-mysql-org/go-mysql/canal"
)

// handleOutboxEvent 把 outbox 表的新事件行交给中继（CDC 低延迟路径）。
//
// 返回是否命中该表。命中就必须返回 true，否则会掉进下面的 ES 规则分支，
// 把发件箱的行当成业务数据同步进 ES 索引。
//
// 只处理 INSERT：
//   - 事件行由业务事务插入，INSERT 就是"有新事件"的信号；
//   - 后续 UPDATE（领取/记账）是中继自己写的，不能再触发一次投递。
func (r *River) handleOutboxEvent(e *canal.RowsEvent) bool {
	if e == nil || e.Table == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(e.Table.Name), outboxrelay.OutboxTableName) {
		return false
	}
	if e.Action != canal.InsertAction {
		return true
	}

	relay := outboxrelay.Current()
	if relay == nil {
		// 中继还没启动（或本次没装配）：不报错，靠中继自己的补偿扫描捞回。
		return true
	}

	for _, id := range collectRowsIDs(e, "id") {
		relay.EnqueueID(id)
	}
	return true
}
