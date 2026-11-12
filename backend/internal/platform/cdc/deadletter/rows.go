package deadletter

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// Record 是 cdc_dead_letter 表的行。
//
// 导出是因为管理端接口会直接读检索结果；表名沿用历史命名（没有 _models 后缀）。
//
// 注意它**没有 DeletedAt**：死信是"出问题的事件快照"，只做保留期清理、不做软删，
// 所以不能套 mysql.BaseRow（那会多出一个 deleted_at 列，与真表对不上）。
type Record struct {
	ID ctype.ID `gorm:"primaryKey;autoIncrement:false" json:"id"`

	Stream      string `gorm:"size:32;not null;index:idx_cdc_dlq_stream_status_created,priority:1;uniqueIndex:uk_cdc_dlq_stream_job,priority:1" json:"stream"`
	CdcJobID    string `gorm:"size:255;not null;uniqueIndex:uk_cdc_dlq_stream_job,priority:2;index:idx_cdc_dlq_job_id" json:"cdc_job_id"`
	SourceTable string `gorm:"size:128;not null" json:"source_table"`
	Action      string `gorm:"size:32;not null" json:"action"`
	TargetKey   string `gorm:"size:255" json:"target_key"`
	PayloadJSON string `gorm:"type:longtext;not null" json:"payload_json"`
	RetryCount  int    `gorm:"default:0;not null" json:"retry_count"`
	Status      string `gorm:"size:32;not null;index:idx_cdc_dlq_stream_status_created,priority:2" json:"status"`
	ErrorCode   string `gorm:"size:64" json:"error_code"`
	ErrorMsg    string `gorm:"type:text" json:"error_msg"`

	CreatedAt time.Time `gorm:"index:idx_cdc_dlq_stream_status_created,priority:3" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (Record) TableName() string { return "cdc_dead_letter" }

// BeforeCreate 生成雪花主键。
//
// 这一条以前漏了，后果很隐蔽：主键恒为 0，而 `ByIDs` / `MarkResolved` /
// `BumpRetry` 全都按主键定位 —— 重投时把空串塞进 `id IN (?)` 与 bigint 比较，
// MySQL 直接报 `Error 1292 Truncated incorrect INTEGER value: ''`。
// 而且主键是唯一键，第二条死信写进来就会撞车。
//
// 为什么一直没被发现：`testkit/schema` 里的 CdcDeadLetterRow **有这个钩子**，
// 用例插进去的行主键都正常，只有生产结构体没有 —— 测试绿、线上坏。
// `rows_test.go` 里有一条用例专门盯着这一点。
func (r *Record) BeforeCreate(*gorm.DB) error {
	if !r.ID.IsZero() {
		return nil
	}
	next, err := idgen.Next()
	if err != nil {
		return err
	}
	r.ID = next
	return nil
}
