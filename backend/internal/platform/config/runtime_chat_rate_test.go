package config

import (
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
)

// 后台「站点 → 私信」的两个限流参数必须有服务端兜底：配置里删键、填 0、填负数
// 时，GetRuntimeChat 仍要给出可用的 10 秒 / 99 条，而不是让限流静默失效。
func TestGetRuntimeChatRateDefaultsAndNormalization(t *testing.T) {
	t.Run("配置缺省时回落到 ADR-0026 默认", func(t *testing.T) {
		svc := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), nil)
		chat := svc.GetRuntimeChat()
		w, d := NormalizeChatRate(chat.RateWindowSec, chat.RateDailyMax).Pair()
		if w != DefaultChatWindowSec || d != DefaultChatDailyMax {
			t.Fatalf("缺省应为 %d 秒 / %d 条，实际 %d 秒 / %d 条",
				DefaultChatWindowSec, DefaultChatDailyMax, w, d)
		}
	})

	t.Run("运行时改值后立即被现读到", func(t *testing.T) {
		svc := NewRuntimeConfigService(conf.RuntimeDefaults{
			Site: conf.Site{Chat: siteconf.Chat{RateWindowSec: 30, RateDailyMax: 200}},
		}, quietLogger(), nil)
		w, d := svc.GetRuntimeChat().RateWindowSec, svc.GetRuntimeChat().RateDailyMax
		if w != 30 || d != 200 {
			t.Fatalf("应现读到 30 秒 / 200 条，实际 %d 秒 / %d 条", w, d)
		}
	})
}
