//go:build integration

package ddl_test

import (
	"testing"

	"myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/mysqlcontainer"
)

// 本文件钉住 #217 的第一条验收：建表命令能创建目录树表，重复执行无副作用。
//
// 零 DDL 口径下新增表只走 CREATE TABLE IF NOT EXISTS；这里连续执行两次，
// 第二次必须同样成功且表结构不变。没有 Docker 时 mysqlcontainer 会 skip。

func TestApplyCreatesOutlineTableIdempotently(t *testing.T) {
	db := mysqlcontainer.DB(t)

	for i := 0; i < 2; i++ {
		if err := ddl.Apply(db); err != nil {
			t.Fatalf("第 %d 次执行建表脚本失败: %v", i+1, err)
		}
	}

	var rows []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
		ColumnType string `gorm:"column:COLUMN_TYPE"`
		Nullable   string `gorm:"column:IS_NULLABLE"`
	}
	if err := db.Raw(
		"SELECT column_name AS COLUMN_NAME, column_type AS COLUMN_TYPE, is_nullable AS IS_NULLABLE "+
			"FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? "+
			"ORDER BY ordinal_position",
		"content_outline_node_models",
	).Scan(&rows).Error; err != nil {
		t.Fatalf("读取目录树表结构失败: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("建表脚本没有创建 content_outline_node_models")
	}

	want := map[string]string{
		"owner_kind": "bigint",
		"owner_id":   "bigint unsigned",
		"parent_id":  "bigint unsigned",
		"kind":       "bigint",
		"title":      "varchar(128)",
		"article_id": "bigint unsigned",
		"sort_order": "bigint",
	}
	got := make(map[string]string, len(rows))
	for _, row := range rows {
		got[row.ColumnName] = row.ColumnType
	}
	for col, typ := range want {
		if got[col] != typ {
			t.Errorf("列 %s 类型应为 %s，实际 %q", col, typ, got[col])
		}
	}
	// 文件夹节点的 article_id 必须是 NULL（唯一索引里 NULL 互不冲突）。
	for _, row := range rows {
		if row.ColumnName == "article_id" && row.Nullable != "YES" {
			t.Errorf("article_id 必须可空（文件夹节点为 NULL），实际 %s", row.Nullable)
		}
	}
}
