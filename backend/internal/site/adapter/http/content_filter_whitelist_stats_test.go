package http

import (
	"testing"

	"myblogx/conf"
)

// 后台拿到的白名单计数必须是"去重后"的口径，与体检命令同一个来源
// （contentfilter.CountWhitelist）。前端自己把两个数组长度相加会得到 50，
// 体检却打印 49 —— 同一个配置两个数字，运维只能猜哪个是真的。

func TestContentFilterSectionResponseReportsWhitelistStats(t *testing.T) {
	t.Parallel()

	response := contentFilterSectionResponse(conf.ContentFilter{
		Words:            []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		Whitelist:        []string{"独立", "证书"},
		WhitelistPhrases: []string{"安全套接层", "独立"},
	})

	if response.WhitelistStats.Total != 3 {
		t.Fatalf("总数应为 3 条（词级 2 + 短语 2 - 跨表重复 1），实际 %d", response.WhitelistStats.Total)
	}
	if response.WhitelistStats.Words != 2 || response.WhitelistStats.Phrases != 2 {
		t.Fatalf("分项应为 2 / 2，实际 %+v", response.WhitelistStats)
	}
	if response.WhitelistStats.Duplicates != 1 {
		t.Fatalf("跨表重复应报 1 条，实际 %+v", response.WhitelistStats)
	}
}
