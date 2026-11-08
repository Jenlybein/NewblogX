package moderation_test

import (
	"testing"

	"myblogx/internal/article/domain/moderation"
)

// 词表分两档：阻断级命中给 high，观察级命中给 medium（票 311 / spec #302 决策 22）。
func TestMatchWordsReportsBothTiers(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{
		Block: []string{"代开发票"},
		Watch: []string{"加微信"},
	}
	got := moderation.MatchWords("本文讲如何代开发票，欢迎加微信。", lists)

	if len(got) != 2 {
		t.Fatalf("命中数应为 2，实际 %d: %+v", len(got), got)
	}
	if got[0].Code != moderation.CodeBlockWord || got[0].Level != moderation.LevelHigh {
		t.Fatalf("第一条应是阻断级 high，实际 %+v", got[0])
	}
	if got[0].Detail != "代开发票" {
		t.Fatalf("阻断级 detail 应是命中的词，实际 %q", got[0].Detail)
	}
	if got[1].Code != moderation.CodeWatchWord || got[1].Level != moderation.LevelMedium {
		t.Fatalf("第二条应是观察级 medium，实际 %+v", got[1])
	}
	if got[1].Detail != "加微信" {
		t.Fatalf("观察级 detail 应是命中的词，实际 %q", got[1].Detail)
	}
}

// 英文词不区分大小写；同一篇里同一个词命中多次只报一条。
func TestMatchWordsIsCaseInsensitiveAndDeduplicated(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{Block: []string{"Spam"}}
	got := moderation.MatchWords("SPAM spam sPam", lists)

	if len(got) != 1 {
		t.Fatalf("同一个词重复出现只应报一条，实际 %d: %+v", len(got), got)
	}
	if got[0].Detail != "Spam" {
		t.Fatalf("detail 应保留配置里的原词，实际 %q", got[0].Detail)
	}
}

// 拉丁字母/数字开头的词必须有词边界：数字词不能命中更长数字的内部。
// 这正是"字符串匹配没有语境"里最有代表性的一类误报（GLOSSARY「阻断级词表」）。
func TestMatchWordsRequiresASCIIWordBoundary(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{Block: []string{"64", "cat"}}

	if got := moderation.MatchWords("编号 1648 的文章", lists); len(got) != 0 {
		t.Fatalf("64 不应命中更长数字 1648，实际 %+v", got)
	}
	if got := moderation.MatchWords("编号 64 的文章", lists); len(got) != 1 || got[0].Detail != "64" {
		t.Fatalf("独立出现的 64 应命中，实际 %+v", got)
	}
	if got := moderation.MatchWords("concat 不是 cat", lists); len(got) != 1 || got[0].Detail != "cat" {
		t.Fatalf("cat 不应命中 concat，但应在独立出现的 cat 命中，实际 %+v", got)
	}
}

// 中日韩文字没有词分隔符：字面命中即可。已知误报（词出现在更长的人名、
// 成语或数字里，例如"六四"出现在"六十四位处理器"之外的长串中）由分档与人工
// 兜底，不在这一层消除（票 311 的既定取舍）。
func TestMatchWordsMatchesCJKSubstrings(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{Watch: []string{"六四"}}
	got := moderation.MatchWords("关于六四的讨论", lists)

	if len(got) != 1 || got[0].Detail != "六四" {
		t.Fatalf("CJK 子串按字面命中，实际 %+v", got)
	}
}

// 空词表、空白词、重复词都不应产生噪声：配置是运营手填的。
func TestMatchWordsSkipsBlankAndDuplicateEntries(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{
		Block: []string{"", "   ", "代开发票", "代开发票"},
		Watch: []string{"代开发票", "加微信"},
	}
	got := moderation.MatchWords("代开发票 加微信", lists)

	if len(got) != 2 {
		t.Fatalf("应只有两条去重后的命中，实际 %d: %+v", len(got), got)
	}
	if got[0].Code != moderation.CodeBlockWord {
		t.Fatalf("同一个词同时出现在两档时应按阻断级报，实际 %+v", got[0])
	}
	if got[1].Code != moderation.CodeWatchWord || got[1].Detail != "加微信" {
		t.Fatalf("第二条应是观察级 加微信，实际 %+v", got[1])
	}
}
