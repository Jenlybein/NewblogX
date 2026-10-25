package imageref

import (
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/platform/mysql"
)

// imageRefRow 是 image_ref_models 的行。
//
// 本包对这张表是**读写**（重建引用 = 先删后插），所以需要完整行：
// 用 Insert 写进去的列必须与真表一致，少一列就会静默丢数据。
type imageRefRow struct {
	mysql.BaseRow

	ImageID  ctype.ID                `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:5"`
	RefType  image_ref_enum.RefType  `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:1"`
	OwnerID  ctype.ID                `gorm:"index;uniqueIndex:uk_image_ref_owner_field_pos,priority:2"`
	Field    image_ref_enum.RefField `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:3"`
	Position int                     `gorm:"default:0;uniqueIndex:uk_image_ref_owner_field_pos,priority:4"`
}

// TableName 固定表名。
func (imageRefRow) TableName() string { return "image_ref_models" }

// imageKeyRow 是"按 object_key 找图片 id"的查询投影。
//
// 只选两列：列名写在 SELECT 里，写错会直接报 Unknown column 而不是静默读空。
type imageKeyRow struct {
	ID        ctype.ID
	ObjectKey string
}

// TableName 固定表名。
func (imageKeyRow) TableName() string { return "image_models" }
