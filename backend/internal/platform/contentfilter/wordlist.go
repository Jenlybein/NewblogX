package contentfilter

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// 本文件是词表的"入口口径"：后台保存（站点用例）与初始词表导入（CLI）都从这里过一遍，
// 保证两处对"什么算一个合法词条"的判断完全一致 —— 否则导入能把后台拒绝的词写进库，
// 下一次后台保存就直接失败，而失败原因看起来与导入毫无关系。
//
// 它与 buildMatcher 的口径刻意重叠但不完全相同：buildMatcher 是"构建引擎时再做一次
// 兜底"（库里可能存着更早版本写进去的数据），这里是"写入前拒绝坏数据"。

// WordError 是一条不合法的词条。
//
// 带上词条本身，是因为批量导入一个 3 万行的文件时，只报"某条档位不认识"等于没报：
// 调用方要么把它拼进错误文案，要么用它定位到文件里的行号。
type WordError struct {
	// Word 是出问题的那条词（已去首尾空白）。
	Word string
	// Reason 是给人看的中文说明。
	Reason string
}

// Error 实现 error。
func (e *WordError) Error() string {
	if e == nil {
		return ""
	}
	return e.Reason
}

// NormalizeWords 整理并校验词表条目。
//
// 四件事：去首尾空白、丢掉空词条（批量导入常带空行）、校验档位/类目/词长，
// 按"小写"去重并保留首次出现的写法。任何一条不合规都整体返回错误 —— 只写一半
// 的词表比不写更难排查。
func NormalizeWords(words []Word) ([]Word, error) {
	out := make([]Word, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, entry := range words {
		word := strings.TrimSpace(entry.Word)
		if word == "" {
			continue
		}
		tier, ok := ParseTier(strings.TrimSpace(string(entry.Tier)))
		if !ok {
			return nil, &WordError{Word: word, Reason: fmt.Sprintf("词条 %q 的档位 %q 不认识", word, entry.Tier)}
		}
		category, ok := ParseCategory(strings.TrimSpace(string(entry.Category)))
		if !ok {
			return nil, &WordError{Word: word, Reason: fmt.Sprintf("词条 %q 的类目 %q 不认识", word, entry.Category)}
		}
		if utf8.RuneCountInString(word) < MinWordRunes {
			return nil, &WordError{Word: word, Reason: fmt.Sprintf("词条 %q 是单字词，单字词不入表", word)}
		}
		// 去重键用 ToLower 而不是 compact：词表是给人看的，两个"看起来一样"的写法
		// 在词云里重复出现只会让运营以为自己看错了。压掉空格的等价判定留给引擎。
		key := strings.ToLower(word)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Word{Word: word, Tier: tier, Category: category})
	}
	return out, nil
}

// NormalizeWhitelist 整理白名单：去首尾空白、丢掉空项、按小写去重（保留首次出现的写法）。
func NormalizeWhitelist(entries []string) []string {
	out := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		key := strings.ToLower(entry)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}
