package mysql

import (
	"encoding/json"
	"time"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// outboxRow 对应 outbox_event_models（事务型发件箱）。
//
// 本包不 import 中继的行结构（它是私有的），但表名必须逐字一致：
// 写侧写错表名，事件永远不会被投递，而且服务照常返回成功。
// 静态守卫见 internal/testkit/schema 的表名契约测试。
type outboxRow struct {
	ID            ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt
	EventName     string
	AggregateType string
	AggregateID   ctype.ID
	Payload       string `gorm:"type:longtext"`
	Status        string `gorm:"size:16"`
	Attempts      int
	AvailableAt   time.Time
	LastError     string `gorm:"size:512"`
	DispatchedAt  *time.Time
}

// TableName 固定表名。
func (outboxRow) TableName() string { return "outbox_event_models" }

// outboxStatusPending 是发件箱事件的初始状态（与中继状态机一致）。
const outboxStatusPending = "pending"

// emitEvent 把一条领域事件写进**当前事务**。
//
// 为什么必须同事务：事件行与业务行要么一起提交、要么一起回滚，这样"消息已落库"
// 与"通知待投递"才不会各自漂移。投递本身不在这里做 —— 由 outbox 中继的订阅者
// 异步完成（见 app/chat_wiring.go 的 chatSubscriber），失败可重试、崩溃可恢复。
func emitEvent(tx *gorm.DB, name string, aggregateID ctype.ID, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	id, err := idgen.Next()
	if err != nil {
		return err
	}
	now := time.Now()
	row := outboxRow{
		ID:            id,
		CreatedAt:     now,
		UpdatedAt:     now,
		EventName:     name,
		AggregateType: domain.OutboxAggregateType,
		AggregateID:   aggregateID,
		Payload:       string(raw),
		Status:        outboxStatusPending,
		AvailableAt:   now,
	}
	return tx.Create(&row).Error
}
