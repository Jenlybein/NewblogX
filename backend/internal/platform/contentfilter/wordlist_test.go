package contentfilter

import (
	"errors"
	"strings"
	"testing"
)

// 词表入口口径：站点保存用例与 CLI 导入共用同一份整理/校验函数，
// 所以这几条规则要在这里钉死（否则两处会各自漂移）。

func TestNormalizeWords整理词条(t *testing.T) {
	got, err := NormalizeWords([]Word{
		{Word: "  刷单  ", Tier: "watch", Category: "fraud"},
		{Word: "刷单", Tier: "block", Category: "drugs"},  // 小写去重：保留首次出现的档位
		{Word: "   ", Tier: "watch", Category: "fraud"}, // 空词条丢掉
		{Word: "PayPal", Tier: "watch", Category: "fraud"},
		{Word: "paypal", Tier: "watch", Category: "fraud"},
	})
	if err != nil {
		t.Fatalf("整理词表失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应只剩 2 条，得到 %+v", got)
	}
	if got[0].Word != "刷单" || got[0].Tier != TierWatch || got[0].Category != CategoryFraud {
		t.Fatalf("去空白与保留首次出现都错了: %+v", got[0])
	}
	if got[1].Word != "PayPal" {
		t.Fatalf("拉丁词应按小写去重并保留首次写法: %+v", got[1])
	}
}

func TestNormalizeWords拒绝非法词条(t *testing.T) {
	cases := map[string]Word{
		"未知档位": {Word: "冰毒", Tier: "blocked", Category: "drugs"},
		"未知类目": {Word: "冰毒", Tier: "block", Category: "weapons"},
		"单字词":  {Word: "毒", Tier: "block", Category: "drugs"},
	}
	for name, word := range cases {
		_, err := NormalizeWords([]Word{word})
		if err == nil {
			t.Fatalf("%s 应被拒绝", name)
		}
		if !strings.Contains(err.Error(), word.Word) {
			t.Fatalf("%s 的错误应带上词条本身，得到 %v", name, err)
		}
		var wordErr *WordError
		if !errors.As(err, &wordErr) {
			t.Fatalf("%s 的错误应是 *WordError（调用方要按类型包错误码），得到 %T", name, err)
		}
	}
}

func TestNormalizeWhitelist去空白去重(t *testing.T) {
	got := NormalizeWhitelist([]string{"  测试  ", "测试", " ", "PayPal", "paypal"})
	if len(got) != 2 || got[0] != "测试" || got[1] != "PayPal" {
		t.Fatalf("白名单整理结果不对: %+v", got)
	}
}
