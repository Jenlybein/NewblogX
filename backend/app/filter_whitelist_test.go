package app

import (
	"testing"

	"myblogx/conf"
)

// 短语级白名单必须进引擎，否则"安全套接层不打星"这类配置就是摆设。
//
// 这条断言看着琐碎，但漏传一份的后果很隐蔽：启动时传了、后台保存后的重建漏传，
// 于是"点一次保存，短语白名单就悄悄失效"——本地起服务怎么试都是好的。

func TestContentFilterWhitelistIncludesPhrases(t *testing.T) {
	t.Parallel()

	merged := contentFilterWhitelist(conf.ContentFilter{
		Whitelist:        []string{"独立", "证书"},
		WhitelistPhrases: []string{"安全套接层", "Fork 炸弹"},
	})
	want := map[string]bool{"独立": true, "证书": true, "安全套接层": true, "Fork 炸弹": true}
	if len(merged) != len(want) {
		t.Fatalf("应合并 4 条，实际 %d 条: %v", len(merged), merged)
	}
	for _, entry := range merged {
		if !want[entry] {
			t.Fatalf("出现了预期外的白名单条目 %q（全部: %v）", entry, merged)
		}
	}
}

func TestContentFilterWhitelistHandlesEmpty(t *testing.T) {
	t.Parallel()

	if got := contentFilterWhitelist(conf.ContentFilter{}); len(got) != 0 {
		t.Fatalf("没有任何白名单时应为空，实际 %v", got)
	}
}
