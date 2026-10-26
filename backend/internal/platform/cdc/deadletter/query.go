package deadletter

import (
	"gorm.io/gorm"
)

// QueryOption 定义死信检索条件。
type QueryOption struct {
	Stream         string
	Status         string
	CdcJobID       string
	CdcJobIDPrefix bool
	CdcJobIDs      []string
	Limit          int
	// Offset 跳过前 N 条（分页用）。0 表示从头开始。
	Offset int
}

// Query 按条件检索死信记录，默认按 created_at 倒序返回。
func Query(db *gorm.DB, option QueryOption) ([]Record, error) {
	if db == nil {
		return nil, nil
	}

	limit := option.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := buildCond(db.Model(&Record{}), option)

	list := make([]Record, 0)
	if option.Offset > 0 {
		query = query.Offset(option.Offset)
	}
	if err := query.Order("created_at DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Count 按同样的条件统计总数（分页要显示"共 N 条"）。
//
// 与 Query 共用 buildCond，避免"列表能查到但总数对不上"这种最难查的偏差。
func Count(db *gorm.DB, option QueryOption) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var total int64
	if err := buildCond(db.Model(&Record{}), option).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// buildCond 拼装检索条件。Query 与 Count 必须走同一份，否则分页总数会与列表不符。
func buildCond(query *gorm.DB, option QueryOption) *gorm.DB {
	if option.Stream != "" {
		query = query.Where("stream = ?", option.Stream)
	}
	if option.Status != "" {
		query = query.Where("status = ?", option.Status)
	}
	if option.CdcJobID != "" {
		if option.CdcJobIDPrefix {
			query = query.Where("cdc_job_id LIKE ?", option.CdcJobID+"%")
		} else {
			query = query.Where("cdc_job_id = ?", option.CdcJobID)
		}
	}
	if len(option.CdcJobIDs) > 0 {
		query = query.Where("cdc_job_id IN ?", option.CdcJobIDs)
	}
	return query
}
