//go:build integration

package mysql

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
)

// 本文件是 #357 的真 MySQL 回归：注册趋势恒为 0 的根因只在
// parseTime=True 的生产驱动下才会出现（SQLite 会把 bug 放绿）。
//
// 运行方式（有 Docker 时自起 MySQL 8；也可设 BLOGX_MYSQL_DSN 指向已有实例）：
//
//	go test -tags integration -run TestSignupsByDay ./internal/analytics/adapter/mysql/
//
// 时区钉在东八区：DATE(created_at) 按会话时区分桶，窗口与断言都用同一时区，
// 否则跨午夜的两行会挪桶。

// signupUserRow 只保留本测试要用的两列，避免把整张用户表的约束拖进夹具。
type signupUserRow struct {
	ID        ctype.ID  `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (signupUserRow) TableName() string { return "user_models" }

func TestSignupsByDayKeysAreDayStrings(t *testing.T) {
	ctx := context.Background()
	db := mysqlcontainer.DB(t, &signupUserRow{})
	stats := NewStats(db)

	shanghai := time.FixedZone("CST", 8*3600)
	day1 := time.Date(2026, 11, 9, 21, 40, 0, 0, shanghai) // 09 日
	day2a := time.Date(2026, 11, 10, 0, 5, 0, 0, shanghai) // 10 日 凌晨
	day2b := time.Date(2026, 11, 10, 23, 55, 0, 0, shanghai)
	outside := time.Date(2026, 11, 3, 12, 0, 0, 0, shanghai) // 窗口外

	// 显式给唯一 ID：signupUserRow 没有嵌 BaseRow，不会走雪花钩子。
	for i, at := range []time.Time{day1, day2a, day2b, outside} {
		if err := db.Create(&signupUserRow{ID: ctype.NewID(uint64(i + 1)), CreatedAt: at}).Error; err != nil {
			t.Fatalf("插入注册行失败: %v", err)
		}
	}

	start := time.Date(2026, 11, 8, 0, 0, 0, 0, shanghai)
	end := time.Date(2026, 11, 11, 0, 0, 0, 0, shanghai)

	got, err := stats.SignupsByDay(ctx, start, end)
	if err != nil {
		t.Fatalf("SignupsByDay 失败: %v", err)
	}

	// 键必须是纯 YYYY-MM-DD —— 生产驱动下曾变成 RFC3339 形态，导致逐日查表 miss。
	for key := range got {
		if len(key) != 10 {
			t.Fatalf("桶键 %q 不是 YYYY-MM-DD 形状（本次修复的核心）", key)
		}
	}
	if got["2026-11-09"] != 1 {
		t.Fatalf("11-09 期望 1 个注册，实际 %d（map=%v）", got["2026-11-09"], got)
	}
	if got["2026-11-10"] != 2 {
		t.Fatalf("11-10 期望 2 个注册（跨 00:05 与 23:55 同一天），实际 %d（map=%v）", got["2026-11-10"], got)
	}
	if len(got) != 2 {
		t.Fatalf("窗口外的那行不应出现，实际桶=%v", got)
	}
}
