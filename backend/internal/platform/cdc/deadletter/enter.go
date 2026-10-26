package deadletter

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Item struct {
	Stream      string
	CdcJobID    string
	SourceTable string
	Action      string
	TargetKey   string
	// Payload 是**各条链路自己的**载荷形状，这里只负责序列化。
	//
	// 类型是 any 而不是 map[string]any：写入端应当传一个与重投端共用的结构体
	// （见 cdc/imageref/payload.go），这样字段名漂移会在编译期就报错。
	// 传 map 也仍然允许 —— 历史代码就是那么写的，序列化结果一样。
	Payload    any
	RetryCount int
	Status     string
	ErrorCode  string
	ErrorMsg   string
}

func SaveBatch(db *gorm.DB, items []Item) error {
	if db == nil || len(items) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			payloadJSON := "{}"
			if item.Payload != nil {
				if byteData, err := json.Marshal(item.Payload); err == nil {
					// nil map / 空结构体会序列化成 "null" / "{}"，统一落成 "{}"：
					// 重投端把 "{}" 当"没有载荷"处理，两种写法不该有两种语义。
					if text := string(byteData); text != "null" && text != "{}" {
						payloadJSON = text
					}
				}
			}
			status := item.Status
			if status == "" {
				status = StatusPending
			}
			record := Record{
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
					"status":       StatusPending,
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
