package writer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDailyAt(t *testing.T) {
	t.Parallel()

	hour, minute, second, err := ParseDailyAt("03:30:00")
	if err != nil {
		t.Fatalf("ParseDailyAt 返回错误: %v", err)
	}
	if hour != 3 || minute != 30 || second != 0 {
		t.Fatalf("解析结果不符合预期: got=%02d:%02d:%02d", hour, minute, second)
	}
}

func TestParseDailyAtInvalid(t *testing.T) {
	t.Parallel()

	cases := []string{"", "99:30:00", "03:70:00", "03:30:99", "3:30", "abc"}
	for _, raw := range cases {
		if _, _, _, err := ParseDailyAt(raw); err == nil {
			t.Fatalf("非法时间 %q 应返回错误", raw)
		}
	}
}

func TestRemoveExpiredDateDirs(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	keepDir := filepath.Join(parent, "2026-04-05")
	expiredDir := filepath.Join(parent, "2026-04-01")
	invalidDir := filepath.Join(parent, "not-a-date")

	for _, dir := range []string{keepDir, expiredDir, invalidDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
	// 非日期目录一律不动：宁可留着，也不能因为名字不像日期就删。
	if _, err := os.Stat(invalidDir); err != nil {
		t.Fatalf("非日期目录应保留: %v", err)
	}
	if _, err := os.Stat(expiredDir); !os.IsNotExist(err) {
		t.Fatalf("过期目录应删除: err=%v", err)
	}
}

// TestCleanupExpiredLogsAcrossSubDirs 覆盖完整入口：
// 五个子目录一起清，保留窗口按"含今天共 N 天"计算。
func TestCleanupExpiredLogsAcrossSubDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// 用固定相对日期构造：今天保留、昨天保留、8 天前删除。
	today := dayStart(time.Now())
	keepToday := today.Format(logDateDirLayout)
	keepYesterday := today.AddDate(0, 0, -1).Format(logDateDirLayout)
	expired := today.AddDate(0, 0, -8).Format(logDateDirLayout)

	for _, sub := range logSubDirs {
		for _, d := range []string{keepToday, keepYesterday, expired} {
			if err := os.MkdirAll(filepath.Join(root, sub, d), 0o755); err != nil {
				t.Fatalf("创建目录失败: %v", err)
			}
		}
	}

	removed, err := CleanupExpiredLogs(root, 2, nil)
	if err != nil {
		t.Fatalf("CleanupExpiredLogs 返回错误: %v", err)
	}
	// 每个子目录应各删掉一个过期目录。
	if removed != len(logSubDirs) {
		t.Fatalf("删除目录数不符: got=%d want=%d", removed, len(logSubDirs))
	}

	for _, sub := range logSubDirs {
		if _, err := os.Stat(filepath.Join(root, sub, keepToday)); err != nil {
			t.Fatalf("今天(%s)的目录应保留: %v", sub, err)
		}
		if _, err := os.Stat(filepath.Join(root, sub, expired)); !os.IsNotExist(err) {
			t.Fatalf("%s 的过期目录应删除: err=%v", sub, err)
		}
	}
}

// TestCleanupExpiredLogsClampsRetention 保留天数小于 1 时按 1 处理，
// 否则会把今天正在写的目录也删掉。
func TestCleanupExpiredLogsClampsRetention(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	todayDir := dayStart(time.Now()).Format(logDateDirLayout)
	for _, sub := range logSubDirs {
		if err := os.MkdirAll(filepath.Join(root, sub, todayDir), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	removed, err := CleanupExpiredLogs(root, 0, nil)
	if err != nil {
		t.Fatalf("CleanupExpiredLogs 返回错误: %v", err)
	}
	if removed != 0 {
		t.Fatalf("保留天数被夹到 1 时不应删除今天的目录，实际删了 %d 个", removed)
	}
	if _, err := os.Stat(filepath.Join(root, logSubDirs[0], todayDir)); err != nil {
		t.Fatalf("今天的目录应保留: %v", err)
	}
}
