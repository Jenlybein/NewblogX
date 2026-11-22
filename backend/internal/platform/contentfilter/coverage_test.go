package contentfilter_test

import (
	"strings"
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// 本文件钉住"类目覆盖自检"：它存在的唯一理由是生产上出现过
// "词库 3 万条、后台看起来很满、但涉政与暴恐两类是 0 条"的静默缺口。
//
// 断言的是可观察结果（缺哪几类、日志那行说什么），不是"内部用了哪个 map"。

// words 造一批同类目的词（数量不重要，只要非 0）。
func words(category contentfilter.Category, n int) []contentfilter.Word {
	out := make([]contentfilter.Word, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, contentfilter.Word{
			Word:     "词条" + string(rune('一'+i)),
			Tier:     contentfilter.TierBlock,
			Category: category,
		})
	}
	return out
}

// TestMissingRequiredCategories 钉住"五类齐全但仍然要报缺两类"。
func TestMissingRequiredCategories(t *testing.T) {
	t.Parallel()

	// 生产实测的分布形状：五类很多、两类为 0。
	complete := append(
		append(
			append(
				append(words(contentfilter.CategoryDrugs, 3), words(contentfilter.CategoryFraud, 5)...),
				words(contentfilter.CategoryPorn, 4)...),
			words(contentfilter.CategoryGambling, 1)...),
		words(contentfilter.CategoryFakeDocs, 2)...)

	missing := contentfilter.MissingRequiredCategories(complete)
	if len(missing) != 2 {
		t.Fatalf("应报缺 2 类，实际 %v", missing)
	}
	if missing[0] != contentfilter.CategoryPolitics || missing[1] != contentfilter.CategoryViolence {
		t.Fatalf("缺的应是 politics, violence，实际 %v", missing)
	}

	// 补上两类之后必须清零：自检不能变成"永远报警"的噪音。
	full := append(complete, append(words(contentfilter.CategoryPolitics, 1), words(contentfilter.CategoryViolence, 1)...)...)
	if got := contentfilter.MissingRequiredCategories(full); len(got) != 0 {
		t.Fatalf("补齐后不该再报缺，实际 %v", got)
	}
}

// TestCategoryCounts 钉住按类目计数（后台提示与运维命令都读它）。
func TestCategoryCounts(t *testing.T) {
	t.Parallel()

	counts := contentfilter.CategoryCounts([]contentfilter.Word{
		{Word: "甲", Category: contentfilter.CategoryDrugs},
		{Word: "乙", Category: contentfilter.CategoryDrugs},
		{Word: "丙", Category: contentfilter.CategoryFraud},
	})
	if counts[contentfilter.CategoryDrugs] != 2 || counts[contentfilter.CategoryFraud] != 1 {
		t.Fatalf("类目计数不对: %v", counts)
	}
	// 没出现的类目不补 0：补 0 会让"没配这一类"与"这一类是 0 条"看起来一样。
	if _, ok := counts[contentfilter.CategoryPolitics]; ok {
		t.Fatalf("未出现的类目不应出现在计数里: %v", counts)
	}
}

// TestFormatCategoryCounts 钉住输出顺序稳定（日志要能两次肉眼对比）。
func TestFormatCategoryCounts(t *testing.T) {
	t.Parallel()

	line := contentfilter.FormatCategoryCounts(map[contentfilter.Category]int{
		contentfilter.CategoryViolence: 0,
		contentfilter.CategoryDrugs:    3672,
	})
	want := "drugs=3672 gambling=0 fraud=0 porn=0 fake_docs=0 politics=0 violence=0"
	if line != want {
		t.Fatalf("格式化顺序不稳定:\n得到 %s\n期望 %s", line, want)
	}
}

// TestCategoryCoverageHint 钉住提示里带了下一步动作（否则日志只会让人焦虑）。
func TestCategoryCoverageHint(t *testing.T) {
	t.Parallel()

	if got := contentfilter.CategoryCoverageHint(nil); got != "" {
		t.Fatalf("没缺类目时不该有提示，实际 %q", got)
	}

	hint := contentfilter.CategoryCoverageHint([]contentfilter.Category{
		contentfilter.CategoryViolence,
		contentfilter.CategoryPolitics,
	})
	for _, must := range []string{"politics", "violence", "build-wordlist", "-filter-import"} {
		if !strings.Contains(hint, must) {
			t.Fatalf("提示里应包含 %q，实际 %q", must, hint)
		}
	}
	// 提示只报类目名，不报词条（词表是敏感数据，日志里不能出现任何词）。
	if strings.Contains(hint, "词条\"") {
		t.Fatalf("提示不该包含词条内容: %q", hint)
	}
}
