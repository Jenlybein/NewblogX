package site_test

import (
	"testing"
	"time"

	"myblogx/conf/site"
)

// 并账周期是运行期配置（#279），不是编译期常量。
//
// 这个用例只钉住"配置值 → 周期"这一条换算：缺省与越界都必须回落到
// ADR-0025 认下的 30 分钟，而不是静默变成 0（0 周期会让任务变成忙循环）。
func TestReconcileInterval(t *testing.T) {
	cases := []struct {
		name    string
		minutes int
		want    time.Duration
	}{
		{name: "零值用默认 30 分钟", minutes: 0, want: 30 * time.Minute},
		{name: "负数用默认 30 分钟", minutes: -5, want: 30 * time.Minute},
		{name: "按配置的分钟数生效", minutes: 5, want: 5 * time.Minute},
		{name: "最小 1 分钟", minutes: 1, want: time.Minute},
		{name: "上限一天", minutes: 1440, want: 24 * time.Hour},
		{name: "超过一天钳到一天", minutes: 1441, want: 24 * time.Hour},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := site.Reconcile{IntervalMinutes: c.minutes}.Interval()
			if got != c.want {
				t.Fatalf("Reconcile{IntervalMinutes:%d}.Interval() = %s，期望 %s",
					c.minutes, got, c.want)
			}
		})
	}
}
