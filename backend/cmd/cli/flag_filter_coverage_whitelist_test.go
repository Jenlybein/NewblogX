package cli

import (
	"strings"
	"testing"

	"myblogx/conf"
)

// 体检里的白名单数字必须自洽：总数 = 词级（去重）+ 短语级（去重）- 跨表重复。
//
// 上一版总数用去重后的结果、两个分项却用原始长度，运营按"词 46 + 短语 4"加起来
// 对不上 50，只能猜哪个数是真的。

func TestCoverageReportWhitelistNumbersAddUp(t *testing.T) {
	t.Parallel()

	section := conf.ContentFilter{
		Words:            []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		Whitelist:        []string{"独立", " 证书 ", "证书"},        // 去重后 2 条
		WhitelistPhrases: []string{"安全套接层", "独立"},               // 去重后 2 条，其中"独立"与词级重复
	}
	report := strings.Join(coverageReportLines(section), "\n")

	// 词级 2 + 短语级 2 - 跨表重复 1 = 3 条。
	if !strings.Contains(report, "白名单 3 条") {
		t.Fatalf("白名单总数应为 3 条（去重后），实际报告：%s", report)
	}
	if !strings.Contains(report, "词级 2 + 短语级 2") {
		t.Fatalf("分项应各报去重后的条数，实际报告：%s", report)
	}
	if !strings.Contains(report, "跨表重复 1 条已合并") {
		t.Fatalf("跨表重复要说出来，否则三个数字加起来对不上，实际报告：%s", report)
	}
}
