package cron_service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDailyAt(t *testing.T) {
	t.Parallel()

	hour, minute, second, err := parseDailyAt("03:30:00")
	if err != nil {
		t.Fatalf("parseDailyAt 返回错误: %v", err)
	}
	if hour != 3 || minute != 30 || second != 0 {
		t.Fatalf("解析结果不符合预期: got=%02d:%02d:%02d", hour, minute, second)
	}
}

func TestParseDailyAtInvalid(t *testing.T) {
	t.Parallel()

	if _, _, _, err := parseDailyAt("99:30:00"); err == nil {
		t.Fatal("非法时间应返回错误")
	}
}

func TestRemoveExpiredDateDirs(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	keepDir := filepath.Join(parent, "2026-04-05")
	expiredDir := filepath.Join(parent, "2026-04-01")
	invalidDir := filepath.Join(parent, "not-a-date")

	for _, dir := range []string{keepDir, expiredDir, invalidDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	keepFrom := time.Date(2026, 4, 5, 0, 0, 0, 0, time.Local)
	removed, err := removeExpiredDateDirs(parent, keepFrom)
	if err != nil {
		t.Fatalf("removeExpiredDateDirs 返回错误: %v", err)
	}
	if removed != 1 {
		t.Fatalf("删除目录数量不符合预期: got=%d want=1", removed)
	}

	if _, err := os.Stat(keepDir); err != nil {
		t.Fatalf("应保留目录被误删: %v", err)
	}
	if _, err := os.Stat(invalidDir); err != nil {
		t.Fatalf("非日期目录应保留: %v", err)
	}
	if _, err := os.Stat(expiredDir); !os.IsNotExist(err) {
		t.Fatalf("过期目录应删除: err=%v", err)
	}
}
