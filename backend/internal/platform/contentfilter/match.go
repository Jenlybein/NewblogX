package contentfilter

import (
	"sort"

	"github.com/yihleego/trie"
)

// span 是原文上的一段 rune 区间 [start, end)。
type span struct {
	start int
	end   int
}

// matchSpan 是一条 AC 命中：影子串里的关键词 + 映射回原文的区间。
type matchSpan struct {
	key   string
	spans span
}

// apply 是 Filter 的实现：归一化 → 影子串匹配 → 映射回原文 → 分组 → 替换。
func (m *matcher) apply(text string) Result {
	orig := []rune(text)
	folded := make([]rune, len(orig))
	for i, r := range orig {
		folded[i] = foldRune(r)
	}
	shadow, shadowIndex := buildShadow(folded)
	if len(shadow) == 0 {
		return Result{Masked: text}
	}
	shadowText := string(shadow)

	dictSpans := m.findSpans(m.dict, shadowText, shadowIndex)
	if len(dictSpans) == 0 {
		return Result{Masked: text}
	}
	sortSpans(dictSpans)
	whiteSpans := m.findSpans(m.white, shadowText, shadowIndex)

	hits := make([]Hit, 0, 4)
	position := make(map[string]int, 4)
	mask := make([]span, 0, len(dictSpans))
	for _, hit := range dictSpans {
		meta, ok := m.meta[hit.key]
		if !ok {
			continue
		}
		index, seen := position[hit.key]
		if !seen {
			index = len(hits)
			position[hit.key] = index
			hits = append(hits, Hit{Word: meta.Word, Tier: meta.Tier, Category: meta.Category})
		}
		hits[index].Snippets = append(hits[index].Snippets, snippetOf(orig, hit.spans))
		// 白名单命中仍记录（档位/类目照旧），只是不参与替换：
		// 命中区间被某个白名单词条完整包住时不打星号。
		//
		// 同一个词可能出现多次：只要有一处真的被替换，这条命中就算"已打星"。
		// 只记"是否被替换过"而不是"被替换了几处"：调用方要回答的是
		// "这段文本里有没有被我们改过的该档命中"，处数另有 MaskedSpans。
		if !coveredBy(hit.spans, whiteSpans) {
			mask = append(mask, hit.spans)
			hits[index].Masked = true
		}
	}
	merged := mergeSpans(mask)
	return Result{Masked: replaceRunes(orig, merged), Hits: hits, MaskedSpans: len(merged)}
}

// findSpans 在影子串上跑一次 AC，把命中区间映射回原文。
//
// 影子串里的 [Begin, End) 覆盖原文的 [shadowIndex[Begin], shadowIndex[End-1]+1)：
// 中间被压掉的空白/分隔符也在这个区间里，于是"冰 毒"会被整段替换成 3 个 `*`。
func (m *matcher) findSpans(t *trie.Trie, shadowText string, shadowIndex []int) []matchSpan {
	emits := t.FindAll(shadowText, false)
	if len(emits) == 0 {
		return nil
	}
	out := make([]matchSpan, 0, len(emits))
	for _, emit := range emits {
		if emit.Begin < 0 || emit.End > len(shadowIndex) || emit.Begin >= emit.End {
			continue
		}
		out = append(out, matchSpan{
			key:   emit.Keyword,
			spans: span{start: shadowIndex[emit.Begin], end: shadowIndex[emit.End-1] + 1},
		})
	}
	return out
}

// sortSpans 按出现位置排序：起点升序，同起点时长的在前。
func sortSpans(spans []matchSpan) {
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].spans.start != spans[j].spans.start {
			return spans[i].spans.start < spans[j].spans.start
		}
		return spans[i].spans.end > spans[j].spans.end
	})
}

// coveredBy 判断一段区间是否被任意白名单区间完整包住。
//
// 用"完整包住"而不是"有交集"：白名单是短语级的豁免（"观察这个词所在的短语"），
// 只要有一部分露在白名单之外，就该照常替换。
func coveredBy(target span, whites []matchSpan) bool {
	for _, white := range whites {
		if white.spans.start <= target.start && white.spans.end >= target.end {
			return true
		}
	}
	return false
}

// snippetOf 截取命中位置前后各 SnippetContextRunes 字的上下文。
//
// Start / End 是命中词在返回的 Text 内的 rune 下标；文首命中时 Start 为 0，
// 文末命中时 End 等于 Text 的 rune 长度 —— 两边都不补省略号，交给渲染层。
func snippetOf(orig []rune, hit span) Snippet {
	start := hit.start - SnippetContextRunes
	if start < 0 {
		start = 0
	}
	end := hit.end + SnippetContextRunes
	if end > len(orig) {
		end = len(orig)
	}
	return Snippet{
		Start: hit.start - start,
		End:   hit.end - start,
		Text:  string(orig[start:end]),
	}
}

// mergeSpans 合并重叠与相邻区间（并集），避免同一段被反复覆盖。
func mergeSpans(spans []span) []span {
	if len(spans) == 0 {
		return nil
	}
	sorted := make([]span, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end < sorted[j].end
	})

	merged := make([]span, 0, len(sorted))
	current := sorted[0]
	for _, next := range sorted[1:] {
		if next.start > current.end {
			merged = append(merged, current)
			current = next
			continue
		}
		if next.end > current.end {
			current.end = next.end
		}
	}
	return append(merged, current)
}

// replaceRunes 把给定区间里的 rune 换成 `*`，长度不变。
func replaceRunes(orig []rune, spans []span) string {
	if len(spans) == 0 {
		return string(orig)
	}
	out := make([]rune, len(orig))
	copy(out, orig)
	for _, s := range spans {
		start := s.start
		if start < 0 {
			start = 0
		}
		end := s.end
		if end > len(out) {
			end = len(out)
		}
		for i := start; i < end; i++ {
			out[i] = '*'
		}
	}
	return string(out)
}
