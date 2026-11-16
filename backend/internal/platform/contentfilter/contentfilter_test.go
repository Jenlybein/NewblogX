package contentfilter_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"myblogx/internal/platform/contentfilter"
)

// 引擎的行为接缝只有两个：输入文本 + 词表进来，替换后的文本与命中明细出来。
// 所以这里全部是表驱动：每条用例只断言"哪几个字被打了星号"和"命中了哪些词"，
// 不看内部用了哪个 trie 节点、也不调私有函数。

func word(text string, tier contentfilter.Tier, category contentfilter.Category) contentfilter.Word {
	return contentfilter.Word{Word: text, Tier: tier, Category: category}
}

// summarize 把命中明细压成 `词/档位/类目/出现次数` 的可比字符串。
func summarize(hits []contentfilter.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, fmt.Sprintf("%s/%s/%s/%d", hit.Word, hit.Tier, hit.Category, len(hit.Snippets)))
	}
	return out
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name      string
		words     []contentfilter.Word
		whitelist []string
		text      string
		masked    string
		hits      []string
	}{
		{
			name:   "普通命中按等长星号替换",
			words:  []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			text:   "这里卖冰毒",
			masked: "这里卖**",
			hits:   []string{"冰毒/block/drugs/1"},
		},
		{
			name: "多个词同时命中，各自一条记录",
			words: []contentfilter.Word{
				word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs),
				word("枪支", contentfilter.TierBlock, contentfilter.CategoryViolence),
			},
			text:   "冰毒与枪支",
			masked: "**与**",
			hits:   []string{"冰毒/block/drugs/1", "枪支/block/violence/1"},
		},
		{
			name: "重叠命中：短词与长词都记录，替换取并集",
			words: []contentfilter.Word{
				word("加微信", contentfilter.TierWatch, contentfilter.CategoryFraud),
				word("加微信付款", contentfilter.TierBlock, contentfilter.CategoryFraud),
			},
			text:   "加微信付款",
			masked: "*****",
			hits:   []string{"加微信付款/block/fraud/1", "加微信/watch/fraud/1"},
		},
		{
			name: "部分重叠的词也各记一条，替换仍是等长并集",
			words: []contentfilter.Word{
				word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs),
				word("毒枭", contentfilter.TierBlock, contentfilter.CategoryDrugs),
			},
			text:   "冰毒枭",
			masked: "***",
			hits:   []string{"冰毒/block/drugs/1", "毒枭/block/drugs/1"},
		},
		{
			name:   "拉丁词大小写不敏感",
			words:  []contentfilter.Word{word("PayPal", contentfilter.TierWatch, contentfilter.CategoryFraud)},
			text:   "PAYPAL 收款",
			masked: "****** 收款",
			hits:   []string{"PayPal/watch/fraud/1"},
		},
		{
			name:   "全角字母与数字折算成半角后仍命中",
			words:  []contentfilter.Word{word("pay123", contentfilter.TierWatch, contentfilter.CategoryFraud)},
			text:   "ｐａｙ１２３",
			masked: "******",
			hits:   []string{"pay123/watch/fraud/1"},
		},
		{
			name:   "零宽字符插在词中间仍然命中，且一起替换",
			words:  []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			text:   "冰\u200b毒",
			masked: "***",
			hits:   []string{"冰毒/block/drugs/1"},
		},
		{
			name:   "插入空格与分隔符仍然命中，被插入的字符同样替换",
			words:  []contentfilter.Word{word("办假证", contentfilter.TierBlock, contentfilter.CategoryFakeDocs)},
			text:   "办 假-证",
			masked: "*****",
			hits:   []string{"办假证/block/fake_docs/1"},
		},
		{
			name:      "白名单命中照常记录但不替换",
			words:     []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			whitelist: []string{"冰毒"},
			text:      "冰毒科普",
			masked:    "冰毒科普",
			hits:      []string{"冰毒/block/drugs/1"},
		},
		{
			name:      "白名单只豁免被短语包住的那次命中，其它位置照常替换",
			words:     []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			whitelist: []string{"冰毒科普"},
			text:      "冰毒科普，冰毒有害",
			masked:    "冰毒科普，**有害",
			hits:      []string{"冰毒/block/drugs/2"},
		},
		{
			name:      "白名单里的符号不影响短语豁免（C++编程）",
			words:     []contentfilter.Word{word("编程", contentfilter.TierWatch, contentfilter.CategoryFraud)},
			whitelist: []string{"C++编程"},
			text:      "C++编程入门",
			masked:    "C++编程入门",
			hits:      []string{"编程/watch/fraud/1"},
		},
		{
			name:   "单字词不入表，不会命中",
			words:  []contentfilter.Word{word("毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			text:   "有毒的东西",
			masked: "有毒的东西",
			hits:   nil,
		},
		{
			name:   "同一个词出现两次只产生一条记录、两段上下文",
			words:  []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			text:   "冰毒。又是冰毒。",
			masked: "**。又是**。",
			hits:   []string{"冰毒/block/drugs/2"},
		},
		{
			name:   "没有命中时原样返回",
			words:  []contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
			text:   "一段很普通的文本。",
			masked: "一段很普通的文本。",
			hits:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := contentfilter.NewEngine(tc.words, tc.whitelist)
			if err != nil {
				t.Fatalf("构建引擎失败: %v", err)
			}
			got := engine.Filter(tc.text)

			if got.Masked != tc.masked {
				t.Fatalf("替换结果不对\n  输入: %q\n  期望: %q\n  实际: %q", tc.text, tc.masked, got.Masked)
			}
			if n, want := utf8.RuneCountInString(got.Masked), utf8.RuneCountInString(tc.text); n != want {
				t.Fatalf("替换必须等长：输入 %d 个字符，输出 %d 个字符", want, n)
			}
			if summary := summarize(got.Hits); !equalStrings(summary, tc.hits) {
				t.Fatalf("命中明细不对\n  期望: %v\n  实际: %v", tc.hits, summary)
			}
		})
	}
}

// 上下文片段：±20 字，文首/文末自然截断，Start/End 是命中词在片段内的下标。
func TestSnippetContext(t *testing.T) {
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
		nil,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	filler := strings.Repeat("一", 30)

	t.Run("文首命中：片段从 0 开始，结束于命中后 20 字", func(t *testing.T) {
		got := engine.Filter("冰毒" + filler)
		snippet := got.Hits[0].Snippets[0]
		if snippet.Start != 0 || snippet.End != 2 {
			t.Fatalf("命中下标应为 0..2，实际 %d..%d", snippet.Start, snippet.End)
		}
		if want := "冰毒" + strings.Repeat("一", 20); snippet.Text != want {
			t.Fatalf("片段文本不对\n  期望: %q\n  实际: %q", want, snippet.Text)
		}
	})

	t.Run("文末命中：片段向前取 20 字，结束于文末", func(t *testing.T) {
		got := engine.Filter(filler + "冰毒")
		snippet := got.Hits[0].Snippets[0]
		if snippet.Start != 20 || snippet.End != 22 {
			t.Fatalf("命中下标应为 20..22，实际 %d..%d", snippet.Start, snippet.End)
		}
		if want := strings.Repeat("一", 20) + "冰毒"; snippet.Text != want {
			t.Fatalf("片段文本不对\n  期望: %q\n  实际: %q", want, snippet.Text)
		}
	})

	t.Run("文中命中：前后各 20 字", func(t *testing.T) {
		got := engine.Filter(filler + "冰毒" + filler)
		snippet := got.Hits[0].Snippets[0]
		if snippet.Start != 20 || snippet.End != 22 {
			t.Fatalf("命中下标应为 20..22，实际 %d..%d", snippet.Start, snippet.End)
		}
		if n := utf8.RuneCountInString(snippet.Text); n != 42 {
			t.Fatalf("片段应有 20+2+20=42 个字符，实际 %d", n)
		}
		// 片段内的下标必须真的指向命中词本身（渲染层直接用它高亮）。
		runes := []rune(snippet.Text)
		if string(runes[snippet.Start:snippet.End]) != "冰毒" {
			t.Fatalf("片段内下标不指向命中词: %q", string(runes[snippet.Start:snippet.End]))
		}
	})
}

// 构建失败必须是错误而不是"少几条词"：启动方要把它变成启动失败。
func TestNewEngineRejectsUnknownTierOrCategory(t *testing.T) {
	if _, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: "blocked", Category: contentfilter.CategoryDrugs}}, nil,
	); err == nil {
		t.Fatal("档位不认识时应当返回错误")
	}
	if _, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: "weapons"}}, nil,
	); err == nil {
		t.Fatal("类目不认识时应当返回错误")
	}
}

// 热替换：新词表立即生效；坏词表不生效、也不把正在用的词表弄丢。
func TestRebuildSwitchesAtomically(t *testing.T) {
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{word("冰毒", contentfilter.TierBlock, contentfilter.CategoryDrugs)},
		nil,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	if got := engine.Filter("冰毒"); got.Masked != "**" {
		t.Fatalf("初始词表应命中，实际 %q", got.Masked)
	}

	if err := engine.Rebuild(
		[]contentfilter.Word{word("枪支", contentfilter.TierBlock, contentfilter.CategoryViolence)},
		nil,
	); err != nil {
		t.Fatalf("重建失败: %v", err)
	}
	if got := engine.Filter("冰毒"); got.Masked != "冰毒" || len(got.Hits) != 0 {
		t.Fatalf("重建后旧词应当不再命中，实际 %q / %v", got.Masked, summarize(got.Hits))
	}
	if got := engine.Filter("枪支"); got.Masked != "**" {
		t.Fatalf("重建后新词应当命中，实际 %q", got.Masked)
	}

	if err := engine.Rebuild(
		[]contentfilter.Word{{Word: "枪支", Tier: "nope", Category: contentfilter.CategoryViolence}},
		nil,
	); err == nil {
		t.Fatal("坏词表重建应当返回错误")
	}
	if got := engine.Filter("枪支"); got.Masked != "**" {
		t.Fatal("重建失败后旧词表必须继续生效（不能切换成半成品）")
	}
}

// 规模烟雾：两万条词的长文过滤仍然线性、结果正确。
// 不断言耗时（CI 抖动会让它变成假红），只断言"规模上去了行为没变"。
func TestScaleTo20kWords(t *testing.T) {
	words := make([]contentfilter.Word, 0, 20000)
	for i := 0; i < 20000; i++ {
		words = append(words, word(fmt.Sprintf("测试词%d号", i), contentfilter.TierWatch, contentfilter.CategoryFraud))
	}
	engine, err := contentfilter.NewEngine(words, []string{"测试词7号"})
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	if got := engine.WordCount(); got != 20000 {
		t.Fatalf("词条数应为 20000，实际 %d", got)
	}

	text := strings.Repeat("这段文字里没有任何登记过的词。", 500) + "测试词1234号"
	got := engine.Filter(text)
	if len(got.Hits) != 1 || got.Hits[0].Word != "测试词1234号" {
		t.Fatalf("应只命中一条，实际 %v", summarize(got.Hits))
	}
	if utf8.RuneCountInString(got.Masked) != utf8.RuneCountInString(text) {
		t.Fatal("替换必须等长")
	}
}
