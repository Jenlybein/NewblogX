package config

import "testing"

// 限流参数由后台「站点 → 私信」下发，非法值必须被收敛，否则填 0 会让
// 限流静默失效（0 秒窗口 = 不限流、0 条日额度 = 全站禁言）。
//
// 默认值与合法区间是生效值模块的一部分（#284），这张表取代原先散在
// conf/site 的逐字段断言：好测试只验生效值模块的外部行为。
func TestNormalizeChatRate(t *testing.T) {
	cases := []struct {
		name                string
		windowSec, dailyMax int
		want                ChatRate
	}{
		{"缺省回落", 0, 0, ChatRate{WindowSec: DefaultChatWindowSec, DailyMax: DefaultChatDailyMax}},
		{"负数回落", -1, -5, ChatRate{WindowSec: DefaultChatWindowSec, DailyMax: DefaultChatDailyMax}},
		{"超上限回落", 999999, 100000, ChatRate{WindowSec: DefaultChatWindowSec, DailyMax: DefaultChatDailyMax}},
		{"合法值原样", 30, 200, ChatRate{WindowSec: 30, DailyMax: 200}},
		{"边界内", 1, 1, ChatRate{WindowSec: 1, DailyMax: 1}},
		{"上界内", 86400, 9999, ChatRate{WindowSec: 86400, DailyMax: 9999}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeChatRate(c.windowSec, c.dailyMax); got != c.want {
				t.Fatalf("NormalizeChatRate(%d,%d) = %+v，期望 %+v",
					c.windowSec, c.dailyMax, got, c.want)
			}
		})
	}
}
