package contentfilter_test

import (
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// 白名单分两份（词级 / 短语级）是给人看的意图区分，引擎只认一张表。
// 合并函数要是漏了一份，后果是"后台保存一次词表之后短语白名单悄悄失效"。

func TestMergeWhitelistJoinsAndDedupes(t *testing.T) {
	t.Parallel()

	merged := contentfilter.MergeWhitelist(
		[]string{"独立", " 证书 ", ""},
		[]string{"安全套接层", "独立", "Fork 炸弹"},
	)
	want := []string{"独立", "证书", "安全套接层", "Fork 炸弹"}
	if len(merged) != len(want) {
		t.Fatalf("合并后应有 %d 条，实际 %d 条: %v", len(want), len(merged), merged)
	}
	for i, expected := range want {
		if merged[i] != expected {
			t.Fatalf("第 %d 条应为 %q，实际 %q（合并顺序与去重都变了: %v）", i, expected, merged[i], merged)
		}
	}
}

func TestMergeWhitelistAcceptsEmpty(t *testing.T) {
	t.Parallel()

	if got := contentfilter.MergeWhitelist(nil, nil); len(got) != 0 {
		t.Fatalf("全空时应返回空表，实际 %v", got)
	}
}
