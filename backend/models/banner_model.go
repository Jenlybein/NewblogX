// 轮播图模型

package models

import "myblogx/models/ctype"

// 轮播图表
type BannerModel struct {
	Model
	Show         bool      `json:"show"`                    // 是否显示
	Cover        string    `json:"cover"`                   // 封面图片链接
	CoverImageID *ctype.ID `gorm:"-" json:"cover_image_id"` // 封面图片 ID，用于前端编辑回填
	Href         string    `json:"href"`                    // 跳转链接
}
