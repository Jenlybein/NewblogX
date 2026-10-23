package cdc_dead_letter_service

import (
	"encoding/json"
	"time"

	"myblogx/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Item struct {
	Stream      string
	CdcJobID    string
	SourceTable string
	Action      string
	TargetKey   string
	Payload     map[string]any
	RetryCount  int
	Status      string
	ErrorCode   string
	ErrorMsg    string
}

func SaveBatch(db *gorm.DB, items []Item) error {
	if db == nil || len(items) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			payloadJSON := "{}"
			if len(item.Payload) > 0 {
				if byteData, err := json.Marshal(item.Payload); err == nil {
					payloadJSON = string(byteData)
				}
			}
			status := item.Status
			if status == "" {
				status = "pending"
			}
			record := models.CdcDeadLetterModel{
				Stream:      item.Stream,
				CdcJobID:    item.CdcJobID,
				SourceTable: item.SourceTable,
				Action:      item.Action,
				TargetKey:   item.TargetKey,
				PayloadJSON: payloadJSON,
				RetryCount:  item.RetryCount,
				Status:      status,
				ErrorCode:   item.ErrorCode,
				ErrorMsg:    item.ErrorMsg,
				UpdatedAt:   time.Now(),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "stream"},
					{Name: "cdc_job_id"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"source_table": record.SourceTable,
					"action":       record.Action,
					"target_key":   record.TargetKey,
					"payload_json": record.PayloadJSON,
					"retry_count":  gorm.Expr("GREATEST(retry_count, ?)", record.RetryCount),
					"status":       "pending",
					"error_code":   record.ErrorCode,
					"error_msg":    record.ErrorMsg,
					"updated_at":   record.UpdatedAt,
				}),
			}).Create(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
