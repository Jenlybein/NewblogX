package outboxrelay

import (
	"time"

	"myblogx/internal/platform/mysql"
)

// OutboxTableName 是发件箱的表名。
//
// 单独导出：river 侧靠表名判断"这条 binlog 是不是发件箱事件"，
// 它不需要行结构，只需要这个名字。两处各写一遍字符串迟早会不一致。
const OutboxTableName = "outbox_event_models"

// 发件箱事件的状态机。
//
//	pending ──中继领取──> processing ──成功──> done
//	                          └──失败──> failed（退避后回到可领取）
//	                                        └──超过最大重试──> dead（进死信）
//
// 为什么要 processing 这个中间态：中继是异步的，"领取"和"处理完"之间进程可能崩溃，
// 留一个中间态，重启后的补偿扫描才知道哪些是"领了但没做完"的。
const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusFailed     = "failed"
	OutboxStatusDead       = "dead"
)

// outboxRow 是 outbox_event_models 的行。
type outboxRow struct {
	mysql.BaseRow

	EventName     string    `gorm:"size:64;not null;index"`
	AggregateType string    `gorm:"size:64;not null"`
	AggregateID   uint64    `gorm:"not null;index"`
	Payload       string    `gorm:"type:longtext"`
	Status        string    `gorm:"size:16;not null;index"`
	Attempts      int       `gorm:"not null;default:0"`
	AvailableAt   time.Time `gorm:"not null;index"`
	LastError     string    `gorm:"size:512"`
	DispatchedAt  *time.Time
}

// TableName 固定表名。
func (outboxRow) TableName() string { return OutboxTableName }
