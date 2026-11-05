//go:build integration

package ddl_test

import (
	"testing"

	"myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/mysqlcontainer"
)

// 本文件钉住「建表脚本能建出目录树表，重复执行无副作用」这条外部行为。
//
// 零 DDL 口径下新增表只走 CREATE TABLE IF NOT EXISTS；这里连续执行两次，
// 第二次必须同样成功且表结构不变。没有 Docker 时 mysqlcontainer 会 skip。
//
// #274 的 expand 阶段把一张共用树表扩成三张：旧的共用表逐字不动，两张按实体
// 命名的树表去掉归属列、唯一键与索引只落在 (owner_id, article_id) 上。
// 断言的是 information_schema 里**真库看到的结构**，不是源码文本。

type outlineTableShape struct {
	table         string
	wantOwnerKind bool
	wantColumns   map[string]string
}

func outlineTableShapes() []outlineTableShape {
	common := map[string]string{
		"owner_id":   "bigint unsigned",
		"parent_id":  "bigint unsigned",
		"kind":       "bigint",
		"title":      "varchar(128)",
		"article_id": "bigint unsigned",
		"sort_order": "bigint",
	}
	shared := make(map[string]string, len(common)+1)
	for col, typ := range common {
		shared[col] = typ
	}
	shared["owner_kind"] = "bigint"
	return []outlineTableShape{
		{
			table:         "content_outline_node_models",
			wantOwnerKind: true,
			wantColumns:   shared,
		},
		{
			table:         "collection_outline_node_models",
			wantOwnerKind: false,
			wantColumns:   common,
		},
		{
			table:         "project_outline_node_models",
			wantOwnerKind: false,
			wantColumns:   common,
		},
	}
}

// TestApplyCreatesOutlineTablesIdempotently 对三张树表逐张验幂等、列类型、
// article_id 可空性，以及两张新表确实没有归属列。
func TestApplyCreatesOutlineTablesIdempotently(t *testing.T) {
	db := mysqlcontainer.DB(t)

	for i := 0; i < 2; i++ {
		if err := ddl.Apply(db); err != nil {
			t.Fatalf("第 %d 次执行建表脚本失败: %v", i+1, err)
		}
	}

	for _, shape := range outlineTableShapes() {
		t.Run(shape.table, func(t *testing.T) {
			var rows []struct {
				ColumnName string `gorm:"column:COLUMN_NAME"`
				ColumnType string `gorm:"column:COLUMN_TYPE"`
				Nullable   string `gorm:"column:IS_NULLABLE"`
			}
			if err := db.Raw(
				"SELECT column_name AS COLUMN_NAME, column_type AS COLUMN_TYPE, is_nullable AS IS_NULLABLE "+
					"FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? "+
					"ORDER BY ordinal_position",
				shape.table,
			).Scan(&rows).Error; err != nil {
				t.Fatalf("读取 %s 结构失败: %v", shape.table, err)
			}
			if len(rows) == 0 {
				t.Fatalf("建表脚本没有创建 %s", shape.table)
			}

			got := make(map[string]string, len(rows))
			for _, row := range rows {
				got[row.ColumnName] = row.ColumnType
			}
			for col, typ := range shape.wantColumns {
				if got[col] != typ {
					t.Errorf("列 %s 类型应为 %s，实际 %q", col, typ, got[col])
				}
			}
			_, hasOwnerKind := got["owner_kind"]
			if hasOwnerKind != shape.wantOwnerKind {
				t.Errorf("%s 是否有归属列 owner_kind：期望 %v，实际 %v",
					shape.table, shape.wantOwnerKind, hasOwnerKind)
			}
			// 文件夹节点的 article_id 必须是 NULL（唯一索引里 NULL 互不冲突）。
			for _, row := range rows {
				if row.ColumnName == "article_id" && row.Nullable != "YES" {
					t.Errorf("article_id 必须可空（文件夹节点为 NULL），实际 %s", row.Nullable)
				}
			}
		})
	}
}
