package models

import (
	"time"

	"myblogx/models/ctype"
	dbservice "myblogx/service/db_service"

	"gorm.io/gorm"
)

// CdcDeadLetterModel 记录 CDC 最终失败（重试耗尽）的死信任务。
type CdcDeadLetterModel struct {
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

func (CdcDeadLetterModel) TableName() string {
	return "cdc_dead_letter"
}

func (m *CdcDeadLetterModel) BeforeCreate(_ *gorm.DB) (err error) {
	if !m.ID.IsZero() {
		return nil
	}
	m.ID, err = dbservice.NextSnowflakeID()
	return err
}
