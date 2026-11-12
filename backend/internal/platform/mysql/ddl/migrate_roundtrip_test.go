//go:build integration

package ddl_test

import (
	"sort"
	"testing"

	"gorm.io/gorm"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/mysqlcontainer"
)

// TestColumnAdditionsMatchSchemaSQL 证明「全新库」与「升级上来的库」得到同一张表。
//
// columnAdditions 与 schema.sql 描述的是同一件事的两条路径：后者给空库建表，
// 前者给已经存在的表补列。任何一处改了类型或可空性而另一处没跟上，就会让
// 全新环境与升级环境长出不同的表 —— 而普通单测只走其中一条，编译器和业务测试
// 都发现不了这种分叉。这条往返测试是唯一能钉住它的地方。
//
// 做法：按 schema.sql 建全表 → 把登记过的列删掉（模拟加列前的老库）→
// 跑一次补列 → 与「从未删列」的库逐列比对。
func TestColumnAdditionsMatchSchemaSQL(t *testing.T) {
	fresh := mysqlcontainer.DB(t)
	if err := mysqlddl.Apply(fresh); err != nil {
		t.Fatalf("schema.sql 建表失败: %v", err)
	}

	upgraded := mysqlcontainer.DB(t)
	if err := mysqlddl.Apply(upgraded); err != nil {
		t.Fatalf("schema.sql 建表失败: %v", err)
	}
	for _, add := range mysqlddl.Additions() {
		// 删列会连带删掉只引用它的索引（MySQL 语义），正好等价于"加列之前的老库"。
		stmt := "ALTER TABLE `" + add.Table + "` DROP COLUMN `" + add.Column + "`"
		if err := upgraded.Exec(stmt).Error; err != nil {
			t.Fatalf("模拟老库删列失败: %v\n语句: %s", err, stmt)
		}
	}
	if err := mysqlddl.ApplyColumnAdditions(upgraded); err != nil {
		t.Fatalf("补列失败: %v", err)
	}

	for _, add := range mysqlddl.Additions() {
		want := columns(t, fresh, add.Table)
		got := columns(t, upgraded, add.Table)
		if d := diffSets(want, got); d != "" {
			t.Errorf("表 %s 在「全新库」与「补列后」之间不一致%s\nschema.sql: %v\n补列后:   %v",
				add.Table, d, want, got)
		}
		if add.UniqueIndex != "" && !indexExists(t, upgraded, add.Table, add.UniqueIndex) {
			t.Errorf("补列后缺唯一索引 %s.%s", add.Table, add.UniqueIndex)
		}
	}
}

// columns 返回 "列名 类型 是否可空" 的三元组，带上类型与可空性而不只是列名。
func columns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var rows []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
		ColumnType string `gorm:"column:COLUMN_TYPE"`
		IsNullable string `gorm:"column:IS_NULLABLE"`
	}
	if err := db.Raw(
		"SELECT column_name AS COLUMN_NAME, column_type AS COLUMN_TYPE, is_nullable AS IS_NULLABLE "+
			"FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?",
		table,
	).Scan(&rows).Error; err != nil {
		t.Fatalf("读 %s 的列失败: %v", table, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ColumnName+" "+r.ColumnType+" "+r.IsNullable)
	}
	sort.Strings(out)
	return out
}

// indexExists 判断索引是否存在。
func indexExists(t *testing.T, db *gorm.DB, table, index string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.statistics "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
		table, index,
	).Scan(&n).Error; err != nil {
		t.Fatalf("读 %s 的索引失败: %v", table, err)
	}
	return n > 0
}

// diffSets 返回两侧差异的可读描述（为空表示一致）。
func diffSets(a, b []string) string {
	inA := make(map[string]bool, len(a))
	for _, s := range a {
		inA[s] = true
	}
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	var onlyA, onlyB []string
	for _, s := range a {
		if !inB[s] {
			onlyA = append(onlyA, s)
		}
	}
	for _, s := range b {
		if !inA[s] {
			onlyB = append(onlyB, s)
		}
	}
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return ""
	}
	out := ""
	if len(onlyA) > 0 {
		out += "\n  只在 schema.sql: " + join(onlyA)
	}
	if len(onlyB) > 0 {
		out += "\n  只在补列后: " + join(onlyB)
	}
	return out
}

// join 拼接切片，避免引入 strings 只为 Join。
func join(items []string) string {
	s := ""
	for i, item := range items {
		if i > 0 {
			s += ", "
		}
		s += item
	}
	return s
}
