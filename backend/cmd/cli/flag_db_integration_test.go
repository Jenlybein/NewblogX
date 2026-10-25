//go:build integration

package cli

import (
	"sort"
	"testing"

	"myblogx/internal/testkit/mysqlcontainer"

	"gorm.io/gorm"
)

// TestFlagDB 在真库上建表，并验证幂等。
//
// 为什么必须是集成测试：FlagDB 现在执行的是 MySQL DDL（internal/platform/mysql/ddl）。
// 以前它跑 AutoMigrate，用 sqlite 也能"通过"—— 但那种通过毫无意义，
// 因为 sqlite 会把 `longtext` / `ENGINE=InnoDB` 这类东西吞掉，
// 线上建不出来的表它照样能建。
func TestFlagDB(t *testing.T) {
	db := mysqlcontainer.DB(t)

	if err := FlagDB(db, nil); err != nil {
		t.Fatalf("FlagDB 返回错误: %v", err)
	}

	tables := listTables(t, db)
	if len(tables) != 32 {
		t.Fatalf("建出 %d 张表，期望 32：%v", len(tables), tables)
	}

	// 抽查几张"最容易漏"的表：它们要么只被某个模块用，要么是后来加的。
	for _, want := range []string{
		"article_models",
		"user_models",
		"chat_session_models",
		"cdc_dead_letter",
		"outbox_event_models",
		"runtime_site_config_models",
	} {
		if !contains(tables, want) {
			t.Errorf("缺表 %s", want)
		}
	}

	// 幂等：再跑一遍不能报错（CREATE TABLE IF NOT EXISTS）。
	if err := FlagDB(db, nil); err != nil {
		t.Fatalf("第二次 FlagDB 应当幂等，实际报错: %v", err)
	}
	if again := listTables(t, db); len(again) != len(tables) {
		t.Fatalf("第二次执行后表数变了: %d → %d", len(tables), len(again))
	}
}

// TestFlagDBRejectsNilDB 确认没传库时给出错误而不是 panic。
func TestFlagDBRejectsNilDB(t *testing.T) {
	if err := FlagDB(nil, nil); err == nil {
		t.Fatal("db 为 nil 时应当返回错误")
	}
}

// listTables 列出当前 schema 的表。
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

// contains 判断切片里有没有目标值。
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
