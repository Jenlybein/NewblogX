package mysql

import "testing"

// 本文件是 #357 的无 Docker 守卫：锁住 SignupsByDay 的键形状。
//
// 真正的回归测试要真 MySQL（见 stats_signups_integration_test.go），但 SQLite 与
// 纯 Go 单测都会把 bug 放绿：SQLite 的 DATE() 返回纯日期，而生产驱动
// （parseTime=True）让 GORM 拿到 RFC3339 形态。下面把两种形态都钉住。

// TestNormalizeDayKeyKeepsPlainDate 纯日期（SQLite 口径）原样保留。
func TestNormalizeDayKeyKeepsPlainDate(t *testing.T) {
	if got := normalizeDayKey("2026-11-11"); got != "2026-11-11" {
		t.Fatalf("normalizeDayKey(纯日期) = %q，期望 2026-11-11", got)
	}
}

// TestNormalizeDayKeyStripsDriverTimestamp 生产驱动的 RFC3339 形态要截回日期。
//
// 这是"注册趋势恒为 0"的直接成因：键变成 "2026-11-11T00:00:00+08:00"，
// 调用方按 "2026-11-11" 查表永远 miss。
func TestNormalizeDayKeyStripsDriverTimestamp(t *testing.T) {
	cases := map[string]string{
		"2026-11-11T00:00:00+08:00":     "2026-11-11",
		"2026-11-11T00:00:00Z":          "2026-11-11",
		"2026-11-11T00:00:00.123+08:00": "2026-11-11",
		"2026-11-11 00:00:00":           "2026-11-11",
		"  2026-11-08T00:00:00+08:00  ": "2026-11-08",
	}
	for raw, want := range cases {
		if got := normalizeDayKey(raw); got != want {
			t.Fatalf("normalizeDayKey(%q) = %q，期望 %q", raw, got, want)
		}
	}
}

// TestNormalizeDayKeyRejectsGarbage 认不出的形状返回空串，由调用方丢弃。
//
// 宁可少一个桶，也不要把垃圾键塞进 map —— 后者会让 GrowthDataView 的
// 逐日查表悄悄错位。
func TestNormalizeDayKeyRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "   ", "not-a-date", "2026/11/11"} {
		if got := normalizeDayKey(raw); got != "" {
			t.Fatalf("normalizeDayKey(%q) = %q，期望空串", raw, got)
		}
	}
}

// TestNormalizeDayKeyMergesSameDayAcrossLayouts 同一自然日的不同序列化合并成一个桶。
func TestNormalizeDayKeyMergesSameDayAcrossLayouts(t *testing.T) {
	for _, raw := range []string{"2026-11-11", "2026-11-11T00:00:00+08:00", "2026-11-11 00:00:00"} {
		if got := normalizeDayKey(raw); got != "2026-11-11" {
			t.Fatalf("normalizeDayKey(%q) = %q，期望 2026-11-11", raw, got)
		}
	}
}
