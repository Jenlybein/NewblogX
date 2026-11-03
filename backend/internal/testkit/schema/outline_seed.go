package schema

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
)

// 本文件是「目录树」表在测试侧的行结构与造数据入口。
//
// 单独成文件：rows.go / schema.go 都已接近 R6 的 800 行上限；树表与合集 /
// 项目同域、成组出现，独立成文件也更好找。

// OutlineNodeRow 对应表 content_outline_node_models（合集 / 项目共用的目录树）。
//
// 与 owner 无关：owner_kind 区分合集与项目，parent_id 表达层次，kind 区分
// 文件夹与文章节点。字段与 article/adapter/mysql 的 outlineNodeRow 逐字一致：
// schema_sql_integration_test 会把结构 AutoMigrate 出来的表与 schema.sql 逐列比对。
//
// 唯一键 uk_outline_owner_article 是 (owner_kind, owner_id, article_id)，
// **不含 deleted_at**：软删行仍占着文章键（"移出后再收录"恢复原行）。
// 文件夹节点的 article_id 为 NULL——MySQL 唯一索引里 NULL 互不冲突，否则
// 同一棵树的所有文件夹都会撞这条唯一键。
type OutlineNodeRow struct {
	BaseRow
	OwnerKind int       `gorm:"not null;index:idx_outline_owner,priority:1;uniqueIndex:uk_outline_owner_article,priority:1"`
	OwnerID   ctype.ID  `gorm:"not null;index:idx_outline_owner,priority:2;uniqueIndex:uk_outline_owner_article,priority:2"`
	ParentID  ctype.ID  `gorm:"not null;default:0"`
	Kind      int       `gorm:"not null"`
	Title     string    `gorm:"type:varchar(128);not null"`
	ArticleID *ctype.ID `gorm:"uniqueIndex:uk_outline_owner_article,priority:3"`
	SortOrder int       `gorm:"not null;default:0"`
}

// TableName 固定表名。
func (OutlineNodeRow) TableName() string { return "content_outline_node_models" }

// OutlineNodeSpec 描述要插入的一个树节点。
//
// ArticleID 为 0 表示文件夹节点（落库是 NULL）；非 0 表示文章节点。
type OutlineNodeSpec struct {
	ID        uint64
	OwnerKind int
	OwnerID   uint64
	ParentID  uint64
	Kind      int
	Title     string
	ArticleID uint64
	SortOrder int
	At        time.Time
}

// InsertOutlineNode 插入一个树节点。
//
// 用 map 写而不是结构体：文件夹的 article_id 必须是 NULL，而零值 ctype.ID
// 落库会变成 0——那会同时撞上"一篇文章"的语义和唯一键。
func InsertOutlineNode(t *testing.T, db *gorm.DB, spec OutlineNodeSpec) {
	t.Helper()
	fields := map[string]any{
		"id":         ctype.NewID(spec.ID),
		"created_at": spec.At,
		"updated_at": spec.At,
		"owner_kind": spec.OwnerKind,
		"owner_id":   ctype.NewID(spec.OwnerID),
		"parent_id":  ctype.NewID(spec.ParentID),
		"kind":       spec.Kind,
		"title":      spec.Title,
		"sort_order": spec.SortOrder,
	}
	if spec.ArticleID != 0 {
		fields["article_id"] = ctype.NewID(spec.ArticleID)
	} else {
		fields["article_id"] = nil
	}
	if err := db.Model(&OutlineNodeRow{}).Create(fields).Error; err != nil {
		t.Fatalf("插入树节点 %d 失败: %v", spec.ID, err)
	}
}
