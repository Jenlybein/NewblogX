package site_test

import (
	"testing"

	siteconf "myblogx/conf/site"
)

// 限流参数由后台「站点 → 私信」下发，非法值必须被收敛，否则填 0 会让
// 限流静默失效（0 秒窗口 = 不限流、0 条日额度 = 全站禁言）。
func TestNormalizeChatRate(t *testing.T) {
	cases := []struct {
		name                string
		windowSec, dailyMax int
		wantW, wantD        int
	}{
		{"缺省回落", 0, 0, 10, 99},
		{"负数回落", -1, -5, 10, 99},
		{"超上限回落", 999999, 100000, 10, 99},
		{"合法值原样", 30, 200, 30, 200},
		{"边界内", 1, 1, 1, 1},
		{"上界内", 86400, 9999, 86400, 9999},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotW, gotD := siteconf.NormalizeChatRate(c.windowSec, c.dailyMax)
			if gotW != c.wantW || gotD != c.wantD {
				t.Fatalf("NormalizeChatRate(%d,%d) = (%d,%d)，期望 (%d,%d)",
					c.windowSec, c.dailyMax, gotW, gotD, c.wantW, c.wantD)
			}
		})
	}
}
