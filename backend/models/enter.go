// 模型模块基础定义

package models

import (
	"time"

	"myblogx/models/ctype"
	dbservice "myblogx/service/db_service"

	"gorm.io/gorm"
)

// 基础模型
type Model struct {
	ID        ctype.ID       `gorm:"primaryKey;autoIncrement:false" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at"`
}

func (m *Model) BeforeCreate(tx *gorm.DB) (err error) {
	if !m.ID.IsZero() {
		return nil
	}
	m.ID, err = dbservice.NextSnowflakeID()
	return err
}

// ID请求参数
type IDRequest struct {
	ID ctype.ID `json:"id" form:"id" uri:"id"`
}

type IDListRequest struct {
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

type OptionsResponse[T any] struct {
	ID    T      `json:"id"`
	Title string `json:"title"`
	Label string `json:"label"`
	Value T      `json:"value"`
}
