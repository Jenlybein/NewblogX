package http

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 时间窗是"页面上一排数字准不准"的唯一口径，边界写错一天就会让运营看到
// 一个永远差一天的趋势图，所以这里按固定时刻把每个窗口的起止与桶数钉死。

func dayAt(t *testing.T, y int, m time.Month, d, hour int) time.Time {
	t.Helper()
	return time.Date(y, m, d, hour, 0, 0, 0, statsTimezone)
}

func TestWindowOf_滚动窗口边界(t *testing.T) {
	// 2026-11-12 是周四；用"深夜"时刻验证切天用的是东八区而不是 UTC。
	now := dayAt(t, 2026, 11, 12, 23)

	cases := []struct {
		raw     string
		buckets int
		first   string
		last    string
		span    int
	}{
		// 向前取 7 天，含今天。
		{"week", 7, "2026-11-06", "2026-11-12", 7},
		// 向前取 30 天，含今天。
		{"month", 30, "2026-10-14", "2026-11-12", 30},
		// today 仍被后端接受（1 天），只是后台不再提供这个按钮。
		{"today", 1, "2026-11-12", "2026-11-12", 1},
	}
	for _, tc := range cases {
		w := windowOf(tc.raw, now)
		if w.Buckets != tc.buckets {
			t.Fatalf("%s 桶数 got=%d want=%d", tc.raw, w.Buckets, tc.buckets)
		}
		if got := w.BucketLabel(0); got != tc.first {
			t.Fatalf("%s 首个桶 got=%s want=%s", tc.raw, got, tc.first)
		}
		if got := w.BucketLabel(w.Buckets - 1); got != tc.last {
			t.Fatalf("%s 末个桶 got=%s want=%s", tc.raw, got, tc.last)
		}
		if got := w.SpanDays(); got != tc.span {
			t.Fatalf("%s 跨度 got=%d want=%d", tc.raw, got, tc.span)
		}
		// 右端固定是"明天 0 点"：今天没过完，但今天的量要算进来。
		wantEnd := dayAt(t, 2026, 11, 13, 0)
		if !w.End.Equal(wantEnd) {
			t.Fatalf("%s 终点 got=%s want=%s", tc.raw, w.End, wantEnd)
		}
	}
}

func TestWindowOf_今年按月出桶(t *testing.T) {
	now := dayAt(t, 2026, 11, 12, 10)
	w := windowOf("year", now)

	if w.Buckets != 12 {
		t.Fatalf("year 应按月出 12 个桶，实际 %d", w.Buckets)
	}
	if got := w.BucketLabel(0); got != "2025-12" {
		t.Fatalf("首个桶应是含本月在内往前第 12 个月，实际 %s", got)
	}
	if got := w.BucketLabel(w.Buckets - 1); got != "2026-11" {
		t.Fatalf("末个桶应是本月，实际 %s", got)
	}
	// 桶边界是整月，不是 30 天。
	if !w.BucketStart(11).Equal(dayAt(t, 2026, 11, 1, 0)) {
		t.Fatalf("最后一个桶的起点应是本月 1 日 0 点，实际 %s", w.BucketStart(11))
	}
	if !w.BucketEnd(11).Equal(dayAt(t, 2026, 12, 1, 0)) {
		t.Fatalf("最后一个桶的终点应是下月 1 日 0 点，实际 %s", w.BucketEnd(11))
	}
	// 12 个自然月的跨度可能是 365 或 366 天。
	if span := w.SpanDays(); span < 365 || span > 366 {
		t.Fatalf("year 跨度应在 365/366 天之间，实际 %d", span)
	}
}

func TestWindowOf_非法取值回落week(t *testing.T) {
	now := dayAt(t, 2026, 11, 12, 10)
	for _, raw := range []string{"", "Yesterday", "d30"} {
		if got := windowOf(raw, now).Range; got != "week" {
			t.Fatalf("%q 应回落 week，实际 %s", raw, got)
		}
	}
}

// 线上事故回归：`/api/data/sum` 曾经漏挂 BindQuery，handler 里用 MustGet 直接
// panic → 500，整张汇总卡变成「汇总读取失败」+ 一排 0。取参数这一步必须容错。
func TestBindQuery_缺绑定中间件时退零值(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got := bindQuery[SumRequest](c); got.Range != "" {
		t.Fatalf("没有绑定值时应返回零值，实际 %+v", got)
	}

	c.Set("requestQuery", SumRequest{Range: "month"})
	if got := bindQuery[SumRequest](c); got.Range != "month" {
		t.Fatalf("应读回绑定值，实际 %+v", got)
	}

	// 绑成了别的类型（路由串了）同样退零值，而不是 panic。
	c.Set("requestQuery", GrowthDataRequest{Type: 1})
	if got := bindQuery[SumRequest](c); got.Range != "" {
		t.Fatalf("类型不匹配时应返回零值，实际 %+v", got)
	}
}
