package contentfilter_test

import (
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// 这条用例钉住"短语白名单 vs 词级白名单"的差别——它是这次运营决策的技术依据：
//   - 短语白名单：只放过这一种说法，词本身的其它用法照旧打星（拦截力保住）；
//   - 词级白名单：这个词永远不打星（拦截力整体交出去）。
//
// 两者在引擎里是同一张表、同一套"区间覆盖"语义，差别只在条目有多具体。
// 所以这条用例同时也在保护一个约定：别哪天把短语白名单折叠成"整词豁免"。

func newPhraseEngine(t *testing.T, whitelist []string) *contentfilter.Engine {
	t.Helper()
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{{
			Word:     "炸弹",
			Tier:     contentfilter.TierBlock,
			Category: contentfilter.CategoryViolence,
		}},
		whitelist,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	return engine
}

func TestPhraseWhitelistOnlyExemptsThatPhrase(t *testing.T) {
	t.Parallel()

	engine := newPhraseEngine(t, []string{"fork 炸弹"})

	// 命中白名单短语：不替换。
	if spans := engine.Filter("幸好挡住了 fork 炸弹").MaskedSpans; spans != 0 {
		t.Fatalf("fork 炸弹 在短语白名单里，不该替换，实际替换 %d 处", spans)
	}
	// 同一个词的其它用法：照旧替换 —— 这就是"保住拦截力"。
	if spans := engine.Filter("他在制作炸弹").MaskedSpans; spans == 0 {
		t.Fatal("短语白名单只该豁免那一个说法，真写「炸弹」必须打星")
	}
}

func TestWordWhitelistExemptsEveryOccurrence(t *testing.T) {
	t.Parallel()

	engine := newPhraseEngine(t, []string{"炸弹"})

	// 词级白名单：任何出现都不替换（代价是这个词的拦截力整体交出去）。
	for _, text := range []string{"他在制作炸弹", "幸好挡住了 fork 炸弹"} {
		if spans := engine.Filter(text).MaskedSpans; spans != 0 {
			t.Fatalf("词级白名单应豁免全部出现，%q 实际替换 %d 处", text, spans)
		}
	}
}
