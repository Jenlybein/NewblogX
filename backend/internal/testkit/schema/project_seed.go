package schema

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
)

// 本文件是「项目」表在测试侧的行结构与造数据入口。
//
// 单独成文件而不是塞回 rows.go / schema.go：archcheck R6 限定单文件 800 行，
// 两个既有文件都已接近上限；项目表与合集同构、成组出现，独立成文件也更好找。

// ProjectRow 对应表 project_models（项目主表）。
//
// 项目是与合集并列的独立内容实体：结构镜像合集（标题/描述/封面/展示开关/
// 排序/创建者 + 软删），但不共用数据、不共用后台维护页。
// 字段与 article/adapter/mysql 的 projectRow 必须逐字一致：
// schema_sql_integration_test 会把结构 AutoMigrate 出来的表与 schema.sql 逐列比对。
type ProjectRow struct {
	BaseRow
	Title       string `gorm:"type:varchar(128);not null" json:"title"`
	Description string `gorm:"type:varchar(512);not null" json:"description"`
	// Cover 是图片库 URL；空串表示"退回首篇公开文章的封面"，由读模型处理。
	Cover string `gorm:"type:longtext;not null" json:"cover"`
	// IsDisplay 是"是否展示给读者"：关掉后读者侧列表与详情都查不到。
	IsDisplay bool `gorm:"not null;default:1" json:"is_display"`
	// SortOrder 是项目之间的展示顺序（从 1 开始）。与合集同一口径：
	// 序号没有唯一索引，读出时用 (sort_order, id) 兜底。
	SortOrder int      `gorm:"not null;default:0" json:"sort_order"`
	CreatorID ctype.ID `gorm:"not null" json:"creator_id"`
}

// TableName 固定表名。
func (ProjectRow) TableName() string { return "project_models" }

// ProjectSpec 描述要插入的项目行。
//
// 项目与合集同构（标题/描述/封面/展示开关/排序/创建者 + 软删），
// 但两套表不共用数据。
type ProjectSpec struct {
	ID          uint64
	Title       string
	Description string
	Cover       string
	Displayed   bool
	SortOrder   int
	CreatorID   uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SoftDeleted bool
}

// InsertProject 插入一个项目行。
func InsertProject(t *testing.T, db *gorm.DB, spec ProjectSpec) {
	t.Helper()
	id := ctype.NewID(spec.ID)
	// 用 map 写：is_display / cover / description 的零值必须真的落库
	// （"建一个不展示的项目"不能被数据库默认值悄悄改成展示）。
	if err := db.Model(&ProjectRow{}).Create(map[string]any{
		"id":          id,
		"created_at":  spec.CreatedAt,
		"updated_at":  spec.UpdatedAt,
		"title":       spec.Title,
		"description": spec.Description,
		"cover":       spec.Cover,
		"is_display":  spec.Displayed,
		"sort_order":  spec.SortOrder,
		"creator_id":  ctype.NewID(spec.CreatorID),
	}).Error; err != nil {
		t.Fatalf("插入项目 %d 失败: %v", spec.ID, err)
	}
	if spec.SoftDeleted {
		if err := db.Model(&ProjectRow{}).
			Where("id = ?", id).
			Update("deleted_at", spec.UpdatedAt).Error; err != nil {
			t.Fatalf("软删项目 %d 失败: %v", spec.ID, err)
		}
	}
}
