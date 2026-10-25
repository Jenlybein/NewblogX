//go:build integration

package schema_test

import (
	"sort"
	"testing"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// TestGeneratedRowsMatchSchemaSQL 证明"测试建的库"与"生产建的库"是同一个库。
//
// # 为什么这条测试是 M6 ⑤-b 的收尾条件
//
// M6 之后表结构有两个来源：
//
//   - 生产：cmd/cli 执行 internal/platform/mysql/ddl 的 schema.sql（FlagDB）；
//   - 测试：testkit/schema 自持的行结构走 AutoMigrate。
//
// 只要这两个来源不一致，"测试全绿"就毫无意义 —— 测的是测试自己造的表。
// 而且偏差方向很危险：自持行是**从 models 复制**来的，复制时少一个字段、
// 少一个 tag，AutoMigrate 建出来的表就窄一列，业务代码写那一列会静默丢数据。
//
// 所以这里建**两个独立库**，一个只用 SQL 建、一个只用行结构建，然后逐表逐列比对。
// 比"只比列名"更严格：值类型与是否可空也一起比。
func TestGeneratedRowsMatchSchemaSQL(t *testing.T) {
	sqlDB := mysqlcontainer.DB(t)
	if err := mysqlddl.Apply(sqlDB); err != nil {
		t.Fatalf("执行 schema.sql 失败: %v", err)
	}

	rowDB := mysqlcontainer.DB(t)
	schema.Migrate(t, rowDB, schema.All)

	sqlTables := listTables(t, sqlDB)
	rowTables := listTables(t, rowDB)

	if diff := diffSets(sqlTables, rowTables); diff != "" {
		t.Fatalf("表清单不一致%s\nschema.sql: %v\nAutoMigrate: %v", diff, sqlTables, rowTables)
	}
	if len(sqlTables) < 30 {
		t.Fatalf("只建出 %d 张表，schema.sql 可能被截断", len(sqlTables))
	}

	for _, table := range sqlTables {
		want := listColumns(t, sqlDB, table)
		got := listColumns(t, rowDB, table)
		if diff := diffSets(want, got); diff != "" {
			t.Errorf("表 %s 的列不一致%s\nschema.sql: %v\nAutoMigrate: %v", table, diff, want, got)
		}
	}
}

// listTables 列出当前 schema 下的全部表。
func listTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var out []string
	if err := db.Raw(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'",
	).Scan(&out).Error; err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	sort.Strings(out)
	return out
}

// listColumns 返回 "列名 类型 是否可空" 的三元组文本，便于直接比对。
//
// 带上类型与 nullability 而不只是列名：少一个 `not null`、
// 把 varchar(32) 写成 varchar(64)，都会让线上的写入语义变掉，
// 而只看列名是发现不了的。
func listColumns(t *testing.T, db *gorm.DB, table string) []string {
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

// diffSets 返回两侧差异的可读描述（为空表示一致）。
func diffSets(a, b []string) string {
	inA := make(map[string]bool, len(a))
	for _, x := range a {
		inA[x] = true
	}
	inB := make(map[string]bool, len(b))
	for _, x := range b {
		inB[x] = true
	}
	var onlyA, onlyB []string
	for _, x := range a {
		if !inB[x] {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !inA[x] {
			onlyB = append(onlyB, x)
		}
	}
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return ""
	}
	msg := ""
	if len(onlyA) > 0 {
		msg += "\n  只在 schema.sql: " + join(onlyA)
	}
	if len(onlyB) > 0 {
		msg += "\n  只在 AutoMigrate: " + join(onlyB)
	}
	return msg
}

// join 用 "|" 连接，避免长列表刷屏。
func join(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += " | "
		}
		out += s
	}
	return out
}
