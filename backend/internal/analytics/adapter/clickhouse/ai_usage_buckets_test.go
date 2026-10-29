package clickhouse

import (
	"testing"
	"time"
)

// TestBuildAIBuckets_Today 验证今日按小时切 24 桶，且起点是东八区零点。
func TestBuildAIBuckets_Today(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 48, 0, 0, cst)
	buckets, err := buildAIBuckets("today", now)
	if err != nil {
		t.Fatalf("buildAIBuckets 失败: %v", err)
	}
	if len(buckets.Keys) != 24 {
		t.Fatalf("桶数 = %d，期望 24", len(buckets.Keys))
	}
	if buckets.Keys[0] != "2026-10-29 00:00:00" {
		t.Fatalf("首桶 = %q，期望 2026-10-29 00:00:00", buckets.Keys[0])
	}
	if buckets.Labels[0] != "00:00" || buckets.Labels[23] != "23:00" {
		t.Fatalf("标签不对: 首 %q 末 %q", buckets.Labels[0], buckets.Labels[23])
	}
	if buckets.Granularity != "hour" {
		t.Fatalf("粒度 = %q，期望 hour", buckets.Granularity)
	}
}

// TestBuildAIBuckets_TodayUsesShanghai 验证 UTC 时间入参仍按东八区算"今天"。
//
// 这是最容易错、而且错了不报错的一处：容器 TZ 是 UTC，用 time.Local
// 会让今日曲线整体偏 8 小时。
func TestBuildAIBuckets_TodayUsesShanghai(t *testing.T) {
	// UTC 10-28 20:00 = 东八区 10-29 04:00
	now := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	buckets, err := buildAIBuckets("today", now)
	if err != nil {
		t.Fatalf("buildAIBuckets 失败: %v", err)
	}
	if buckets.Keys[0] != "2026-10-29 00:00:00" {
		t.Fatalf("首桶 = %q，期望 2026-10-29 00:00:00（应按东八区算今天）", buckets.Keys[0])
	}
}

// TestBuildAIBuckets_WeekStartsMonday 验证本周从周一起算 7 天。
func TestBuildAIBuckets_WeekStartsMonday(t *testing.T) {
	// 2026-10-29 是周四
	now := time.Date(2026, 10, 29, 14, 48, 0, 0, cst)
	buckets, err := buildAIBuckets("week", now)
	if err != nil {
		t.Fatalf("buildAIBuckets 失败: %v", err)
	}
	if len(buckets.Keys) != 7 {
		t.Fatalf("桶数 = %d，期望 7", len(buckets.Keys))
	}
	if buckets.Keys[0] != "2026-10-26 00:00:00" {
		t.Fatalf("首桶 = %q，期望 2026-10-26（周一）", buckets.Keys[0])
	}
	if buckets.Keys[6] != "2026-11-01 00:00:00" {
		t.Fatalf("末桶 = %q，期望 2026-11-01（周日，跨越月末）", buckets.Keys[6])
	}
	if buckets.Labels[0] != "10-26" {
		t.Fatalf("标签 = %q，期望 10-26", buckets.Labels[0])
	}
}

// TestBuildAIBuckets_WeekOnSunday 验证周日属于"刚过完的这一周"，起点仍是周一。
func TestBuildAIBuckets_WeekOnSunday(t *testing.T) {
	// 2026-11-01 是周日
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, cst)
	buckets, err := buildAIBuckets("week", now)
	if err != nil {
		t.Fatalf("buildAIBuckets 失败: %v", err)
	}
	if buckets.Keys[0] != "2026-10-26 00:00:00" {
		t.Fatalf("首桶 = %q，期望 2026-10-26", buckets.Keys[0])
	}
}

// TestBuildAIBuckets_Month 验证本月按天切，天数随月份变化。
func TestBuildAIBuckets_Month(t *testing.T) {
	cases := []struct {
		now  time.Time
		want int
		tail string
	}{
		{time.Date(2026, 10, 29, 0, 0, 0, 0, cst), 31, "2026-10-31 00:00:00"},
		{time.Date(2026, 11, 5, 0, 0, 0, 0, cst), 30, "2026-11-30 00:00:00"},
		// 闰年 2 月：29 天。用 2024 而不是 2026，专门钉住闰月。
		{time.Date(2024, 2, 10, 0, 0, 0, 0, cst), 29, "2024-02-29 00:00:00"},
		{time.Date(2026, 2, 10, 0, 0, 0, 0, cst), 28, "2026-02-28 00:00:00"},
	}
	for _, tc := range cases {
		buckets, err := buildAIBuckets("month", tc.now)
		if err != nil {
			t.Fatalf("buildAIBuckets 失败: %v", err)
		}
		if len(buckets.Keys) != tc.want {
			t.Fatalf("%s 桶数 = %d，期望 %d", tc.now.Format("2006-01"), len(buckets.Keys), tc.want)
		}
		if buckets.Keys[len(buckets.Keys)-1] != tc.tail {
			t.Fatalf("末桶 = %q，期望 %q", buckets.Keys[len(buckets.Keys)-1], tc.tail)
		}
	}
}

// TestBuildAIBuckets_Year 验证本年按月切 12 桶。
func TestBuildAIBuckets_Year(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, cst)
	buckets, err := buildAIBuckets("year", now)
	if err != nil {
		t.Fatalf("buildAIBuckets 失败: %v", err)
	}
	if len(buckets.Keys) != 12 {
		t.Fatalf("桶数 = %d，期望 12", len(buckets.Keys))
	}
	if buckets.Keys[0] != "2026-01-01 00:00:00" || buckets.Keys[11] != "2026-12-01 00:00:00" {
		t.Fatalf("首尾桶不对: %q / %q", buckets.Keys[0], buckets.Keys[11])
	}
	if buckets.Labels[0] != "2026-01" {
		t.Fatalf("标签 = %q，期望 2026-01", buckets.Labels[0])
	}
}

// TestBuildAIBuckets_UnknownRange 验证非法周期直接报错，不返回空时间轴。
func TestBuildAIBuckets_UnknownRange(t *testing.T) {
	if _, err := buildAIBuckets("decade", time.Now()); err == nil {
		t.Fatal("非法周期应当报错")
	}
}
