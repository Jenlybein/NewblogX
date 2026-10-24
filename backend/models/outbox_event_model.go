package models

import "time"

// outbox 事件的状态。
//
// 状态机：
//
//	pending ──中继领取──> processing ──成功──> done
//	                          │
//	                          └──失败──> failed（按退避时间回到可领取状态）
//	                                        │
//	                                        └──超过最大重试──> dead（进死信队列）
//
// 为什么需要 processing 而不是直接从 pending 改 done：
// 中继是异步的，"领取"和"处理完"之间进程可能崩溃；
// 留一个中间态，崩溃重启后的补偿扫描才知道哪些是"领了但没做完"的。
const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusFailed     = "failed"
	OutboxStatusDead       = "dead"
)

// OutboxEventModel 是事务型发件箱（transactional outbox）的事件行。
//
// 为什么要有这张表，而不是在内存里同步分发：
//   - 内存分发在"事务已提交、分发还没做完"时进程崩溃会丢事件；
//   - 内存分发下，下游失败只能让整个业务事务回滚（耦合太强）。
//
// 落表之后：事件行与业务写入在**同一个事务**里提交，
// 于是"事件存在 ⇔ 业务已提交"；投递交给 CDC 中继异步做，
// 崩溃可恢复、失败可重试、重试耗尽进死信。
//
// 注意：新增这张表不违反"零 DDL"约束——该约束只禁止改动/删除**既有**表，
// 因为 ES 投影靠 CDC 按列名解析 binlog；新增表不影响任何既有表的列。
type OutboxEventModel struct {
	Model
	// EventName 事件名，如 "article.published"。
	EventName string `gorm:"size:64;not null;index" json:"event_name"`
	// AggregateType 聚合类型，如 "article"。
	AggregateType string `gorm:"size:64;not null" json:"aggregate_type"`
	// AggregateID 聚合 ID（如文章 ID），便于检索"某篇文章的事件"与人工排查。
	AggregateID uint64 `gorm:"not null;index" json:"aggregate_id"`
	// Payload 事件体（已序列化的 JSON 原文）。
	Payload string `gorm:"type:longtext" json:"payload"`
	// Status 见上面的状态机常量。
	Status string `gorm:"size:16;not null;index" json:"status"`
	// Attempts 已尝试次数，用于退避与判定是否耗尽重试。
	Attempts int `gorm:"not null;default:0" json:"attempts"`
	// AvailableAt 下次可领取的时间（失败后按退避后移）。
	AvailableAt time.Time `gorm:"not null;index" json:"available_at"`
	// LastError 最近一次失败原因，便于排查。
	LastError string `gorm:"size:512" json:"last_error"`
	// DispatchedAt 首次投递成功的时间；为空表示还没成功过。
	DispatchedAt *time.Time `json:"dispatched_at"`
}

// TableName 指定表名。
func (OutboxEventModel) TableName() string { return "outbox_event_models" }
