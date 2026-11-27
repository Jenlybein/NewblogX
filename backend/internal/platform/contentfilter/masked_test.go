package contentfilter_test

import (
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// Hit.Masked 是"这条命中**真的被打星了吗**"的唯一事实来源。
//
// 为什么需要它：白名单覆盖的命中照样进 Hits（要能统计误伤），可那些位置一个字符
// 都没变。上层要判"命中即拒"时必须看这个字段 —— 只看 Tier 会把 `冰毒科普`
// 这种被短语白名单豁免的正常说法一起拒掉，白名单于是形同虚设。
//
// 这里的用例只断言事实，不掺任何处置口径（处置是调用方的事）。

func TestHitMaskedTracksActualReplacement(t *testing.T) {
	words := []contentfilter.Word{
		word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs),
		word("加微信", contentfilter.TierWatch, contentfilter.CategoryFraud),
	}

	tests := []struct {
		name      string
		whitelist []string
		text      string
		word      string
		want      bool
	}{
		{
			name: "block 词命中即打星，Masked 为 true",
			text: "这里卖冰毒",
			word: "冰毒",
			want: true,
		},
		{
			name: "watch 词命中同样打星（两档默认都替换）",
			text: "加微信详聊",
			word: "加微信",
			want: true,
		},
		{
			name:      "block 词被词级白名单完整覆盖：记录仍在，但没打星",
			whitelist: []string{"冰毒"},
			text:      "冰毒科普",
			word:      "冰毒",
			want:      false,
		},
		{
			name:      "block 词被短语白名单完整覆盖：同样不打星",
			whitelist: []string{"冰毒科普"},
			text:      "冰毒科普",
			word:      "冰毒",
			want:      false,
		},
		{
			name:      "同一词两处命中、只有一处被豁免：有一处真打星就算已打星",
			whitelist: []string{"冰毒科普"},
			text:      "冰毒科普，冰毒有害",
			word:      "冰毒",
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := contentfilter.NewEngine(words, tc.whitelist)
			if err != nil {
				t.Fatalf("构建引擎失败: %v", err)
			}
			got := engine.Filter(tc.text)
			if len(got.Hits) == 0 {
				t.Fatalf("应当至少有一条命中记录（白名单命中也要记）: %q", tc.text)
			}
			hit := got.Hits[0]
			if hit.Word != tc.word {
				t.Fatalf("命中的词应为 %q，实际 %q", tc.word, hit.Word)
			}
			if hit.Masked != tc.want {
				t.Fatalf("%q 的 Masked 应为 %v，实际 %v（文本 %q）", tc.word, tc.want, hit.Masked, got.Masked)
			}
			// 交叉验证：Masked 必须与"文本到底变没变"一致，否则它就是一个会漂的副本。
			if changed := got.Masked != tc.text; changed != tc.want {
				t.Fatalf("Masked=%v 与文本是否被替换（%v）不一致：输入 %q 输出 %q",
					hit.Masked, changed, tc.text, got.Masked)
			}
		})
	}
}
