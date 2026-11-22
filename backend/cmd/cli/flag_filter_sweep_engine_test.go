package cli

import (
	"testing"

	"myblogx/conf"
)

// 清洗与在线路径必须用同一套白名单（词级 + 短语级合并）。
//
// 漏了短语级：预演会多报几处假命中；带上 -filter-sweep-apply 就会在真实文章上
// 错打星 —— 这是清洗命令最不该出错的地方，而它只在运维手动跑命令时才走到。

func TestSweepEngineUsesPhraseWhitelist(t *testing.T) {
	t.Parallel()

	engine, err := sweepEngine(conf.ContentFilter{
		Words:            []conf.ContentFilterWord{{Word: "炸弹", Tier: "block", Category: "violence"}},
		WhitelistPhrases: []string{"fork 炸弹"},
	})
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}

	if spans := engine.Filter("挡住 fork 炸弹").MaskedSpans; spans != 0 {
		t.Fatalf("短语白名单应豁免 fork 炸弹，实际替换 %d 处", spans)
	}
	if spans := engine.Filter("他在制作炸弹").MaskedSpans; spans == 0 {
		t.Fatal("短语白名单只该豁免那一个说法，真写「炸弹」仍要替换")
	}
}

func TestSweepEngineUsesWordWhitelist(t *testing.T) {
	t.Parallel()

	engine, err := sweepEngine(conf.ContentFilter{
		Words:     []conf.ContentFilterWord{{Word: "独立", Tier: "block", Category: "politics"}},
		Whitelist: []string{"独立"},
	})
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	if spans := engine.Filter("独立部署与独立进程").MaskedSpans; spans != 0 {
		t.Fatalf("词级白名单应豁免全部出现，实际替换 %d 处", spans)
	}
}
