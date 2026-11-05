package schema

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
)

// 本文件是 #274 expand 期两张**按实体命名**的树表在测试侧的行结构与造数据入口。
//
// 单独成文件：rows.go / schema.go 都已接近 archcheck R6 的 800 行上限；
// 与 outline_seed.go / project_seed.go 的既有做法一致，不塞回大文件。
//
// 两张新表去掉归属列 owner_kind（实体由表名表达），唯一键与索引只落在
// (owner_id, article_id) 上；字段与 article/adapter/mysql 的私有 row 逐字一致，
// schema_sql_integration_test 会把 AutoMigrate 出来的表与 schema.sql 逐列比对。

// CollectionOutlineNodeRow 对应表 collection_outline_node_models（合集目录树）。
type CollectionOutlineNodeRow struct {
	BaseRow
	OwnerID   ctype.ID  `gorm:"not null;index:idx_collection_outline_owner,priority:1;uniqueIndex:uk_collection_outline_owner_article,priority:1"`
	ParentID  ctype.ID  `gorm:"not null;default:0"`
	Kind      int       `gorm:"not null"`
	Title     string    `gorm:"type:varchar(128);not null"`
	ArticleID *ctype.ID `gorm:"index:idx_collection_outline_owner,priority:2;uniqueIndex:uk_collection_outline_owner_article,priority:2"`
	SortOrder int       `gorm:"not null;default:0"`
}

// TableName 固定表名。
func (CollectionOutlineNodeRow) TableName() string { return "collection_outline_node_models" }

// ProjectOutlineNodeRow 对应表 project_outline_node_models（项目目录树）。
type ProjectOutlineNodeRow struct {
	BaseRow
	OwnerID   ctype.ID  `gorm:"not null;index:idx_project_outline_owner,priority:1;uniqueIndex:uk_project_outline_owner_article,priority:1"`
	ParentID  ctype.ID  `gorm:"not null;default:0"`
	Kind      int       `gorm:"not null"`
	Title     string    `gorm:"type:varchar(128);not null"`
	ArticleID *ctype.ID `gorm:"index:idx_project_outline_owner,priority:2;uniqueIndex:uk_project_outline_owner_article,priority:2"`
	SortOrder int       `gorm:"not null;default:0"`
}

// TableName 固定表名。
func (ProjectOutlineNodeRow) TableName() string { return "project_outline_node_models" }

// NamedOutlineNodeSpec 描述要插入的一个树节点（两张新表共用同一组字段）。
//
// ArticleID 为 0 表示文件夹节点（落库是 NULL）；非 0 表示文章节点。
type NamedOutlineNodeSpec struct {
	ID        uint64
	OwnerID   uint64
	ParentID  uint64
	Kind      int
	Title     string
	ArticleID uint64
	SortOrder int
	At        time.Time
}

// namedOutlineFields 把 spec 转成落库字段：文件夹的 article_id 必须是 NULL
// 而不是零值，否则会撞唯一键并被误读成「指向文章 0」。
func namedOutlineFields(spec NamedOutlineNodeSpec) map[string]any {
	fields := map[string]any{
		"id":         ctype.NewID(spec.ID),
		"created_at": spec.At,
		"updated_at": spec.At,
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
	return fields
}

// InsertCollectionOutlineNode 往合集具名树表播种一行。
func InsertCollectionOutlineNode(t *testing.T, db *gorm.DB, spec NamedOutlineNodeSpec) {
	t.Helper()
	if err := db.Model(&CollectionOutlineNodeRow{}).Create(namedOutlineFields(spec)).Error; err != nil {
		t.Fatalf("插入合集树节点 %d 失败: %v", spec.ID, err)
	}
}

// InsertProjectOutlineNode 往项目具名树表播种一行。
func InsertProjectOutlineNode(t *testing.T, db *gorm.DB, spec NamedOutlineNodeSpec) {
	t.Helper()
	if err := db.Model(&ProjectOutlineNodeRow{}).Create(namedOutlineFields(spec)).Error; err != nil {
		t.Fatalf("插入项目树节点 %d 失败: %v", spec.ID, err)
	}
}
