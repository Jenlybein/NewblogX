package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum/image_ref_enum"
)

// ImageRefModel 记录图片被哪个业务对象引用。
// 图片引用只在业务对象保存成功后写入，不在上传图片时写入。
type ImageRefModel struct {
	Model
	ImageID    ctype.ID                `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:5" json:"image_id"`
	RefType    image_ref_enum.RefType  `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:1" json:"ref_type"`
	OwnerID    ctype.ID                `gorm:"index;uniqueIndex:uk_image_ref_owner_field_pos,priority:2" json:"owner_id"`
	Field      image_ref_enum.RefField `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:3" json:"field"`
	Position   int                     `gorm:"default:0;uniqueIndex:uk_image_ref_owner_field_pos,priority:4" json:"position"`
	ImageModel ImageModel              `gorm:"foreignKey:ImageID;references:ID" json:"image_model"`
}
