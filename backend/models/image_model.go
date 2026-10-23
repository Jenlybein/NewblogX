package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// ImageModel 表示已经通过服务端验收、可供业务引用的正式图片。
// Hash 当前存储的是七牛返回的 etag，用于图片内容去重。
type ImageModel struct {
	Model
	UserID    ctype.ID           `gorm:"index" json:"user_id"`
	Provider  enum.ImageProvider `gorm:"size:16;default:qiniu" json:"provider"`
	Bucket    string             `gorm:"size:64" json:"bucket"`
	ObjectKey string             `gorm:"size:255;uniqueIndex:uk_image_object_key" json:"object_key"`
	FileName  string             `gorm:"size:255" json:"file_name"`
	URL       string             `gorm:"size:512" json:"url"`
	MimeType  string             `gorm:"size:128" json:"mime_type"`
	Size      int64              `gorm:"default:0" json:"size"`
	Width     int                `gorm:"default:0" json:"width"`
	Height    int                `gorm:"default:0" json:"height"`
	Hash      string             `gorm:"size:64;uniqueIndex:uk_image_hash" json:"hash"`
	Status    enum.ImageStatus   `gorm:"default:1" json:"status"`
}

func (i ImageModel) WebPath() string {
	return i.URL
}
