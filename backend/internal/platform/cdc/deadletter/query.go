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

	query := db.Model(&Record{})
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

	list := make([]Record, 0)
	if err := query.Order("created_at DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
