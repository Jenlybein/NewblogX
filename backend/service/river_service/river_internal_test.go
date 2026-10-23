package river_service

import "testing"

func TestIsValidTables(t *testing.T) {
	if isValidTables([]string{"*", "a"}) {
		t.Fatal("包含 * 且多表时应返回 false")
	}
	if !isValidTables([]string{"*"}) {
		t.Fatal("单独 * 应允许")
	}
	if !isValidTables([]string{"a", "b"}) {
		t.Fatal("普通多表应允许")
	}
}

func TestBuildTableAndRuleKey(t *testing.T) {
	if got := buildTable("*"); got != ".*" {
		t.Fatalf("buildTable(*) 错误: %s", got)
	}
	if got := buildTable("users"); got != "users" {
		t.Fatalf("buildTable(users) 错误: %s", got)
	}
	if got := ruleKey("DB", "Users"); got != "db:users" {
		t.Fatalf("ruleKey 错误: %s", got)
	}
}
