// Package contentfilter 是全站唯一的内容过滤引擎（ADR-0032）。
//
// 它是纯函数包：不碰数据库、不碰 HTTP、也不认识"文章 / 评论 / 昵称"这些业务概念。
// 输入一段文本与一份词表，输出"替换后的文本 + 命中明细"，其余判断都在调用方。
//
// 三个刻意选择：
//   - 匹配在"归一化 + 压掉空白与分隔符"的影子串上做，命中区间再映射回原文；
//   - 替换按 rune 等长，命中的原文区间整段变成 `*`（含被插进去的空格与分隔符）；
//   - 白名单命中仍产生 Hit（要能统计误伤），但不替换。
package contentfilter

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/yihleego/trie"
)

// Tier 是词条档位。两档**都替换**，差别只在风险页的标识颜色与处置口径。
type Tier string

const (
	// TierBlock 拦截词：明确指向违法交易/涉政/暴恐等。
	TierBlock Tier = "block"
	// TierWatch 观察词：常见于推广话术，误报代价低。
	TierWatch Tier = "watch"
)

// Category 是词条类目（固定七类，不做自由文本）。
type Category string

const (
	CategoryDrugs    Category = "drugs"
	CategoryGambling Category = "gambling"
	CategoryFraud    Category = "fraud"
	CategoryPorn     Category = "porn"
	CategoryFakeDocs Category = "fake_docs"
	CategoryPolitics Category = "politics"
	CategoryViolence Category = "violence"
)

const (
	// SnippetContextRunes 是命中上下文的前后各多少字（ADR-0032：风险页看 ±20 字）。
	SnippetContextRunes = 20
	// MinWordRunes 是入库词的最短长度：单字词误报太多，一律不入表。
	MinWordRunes = 2
)

// Word 是词表里的一条。Word 字段是**原文写法**（不做归一化），匹配用的是它的影子串。
type Word struct {
	Word     string
	Tier     Tier
	Category Category
}

// Snippet 是一条命中的上下文。
//
// Start / End 是命中词在 Text 内的 rune 下标（左闭右开），调用方直接用它们高亮；
// Text 是原文里命中位置前后各 20 字的截取（文首/文末自然截断）。
// 用"相对 Text 的下标"而不是"相对全文的下标"：风险记录只存 Text 与这条区间，
// 存全文下标会让消费方拿到一份无法自洽的数据。
type Snippet struct {
	Start int
	End   int
	Text  string
}

// Hit 是一个词在本段文本里的全部命中。
//
// 同一个词出现多次只产生一条 Hit，每次出现的上下文收进 Snippets —— 风险页要的是
// "哪个词、在哪出现、上下文是什么"，不是"每个字符区间一条记录"。
type Hit struct {
	Word     string
	Tier     Tier
	Category Category
	Snippets []Snippet
}

// Result 是过滤结果。Masked 永远与输入等长（按 rune 计）。
type Result struct {
	Masked string
	Hits   []Hit
	// MaskedSpans 是**实际被替换的处数**（白名单命中的区间不算，重叠命中按并集算一处）。
	//
	// 调用方拿它回答"已过滤 N 处"：Hits 里有白名单命中（记录了但不替换），
	// 用 len(Hits) 或片段数当 N 会把没打的星号也算进去，对用户是一次撒谎。
	MaskedSpans int
}

// Engine 持有当前生效的匹配器，词表变更后整份原子替换。
//
// 请求路径只读、不构建：构建（含失败）发生在启动与后台保存时。旧匹配器在替换的那一刻
// 之后不再被任何新请求取到，正在跑的请求继续用它自己的那一份 —— 保存不会阻塞请求。
type Engine struct {
	state atomic.Value // *matcher
}

// matcher 是一份不可变的匹配器（构建完成后只读）。
type matcher struct {
	dict  *trie.Trie
	white *trie.Trie
	// meta 是"影子串 → 词条"，用于把 AC 命中的关键词还原成档位与类目。
	meta map[string]Word
}

// NewEngine 用给定词表与白名单构建引擎。
//
// 词表非法（档位/类目不认识）时返回 error 而不是"忽略这一条"：启动方负责把它变成
// 启动失败 —— 无过滤上线等于裸奔（ADR-0032 决策 11）。
func NewEngine(words []Word, whitelist []string) (*Engine, error) {
	m, err := buildMatcher(words, whitelist)
	if err != nil {
		return nil, err
	}
	engine := &Engine{}
	engine.state.Store(m)
	return engine, nil
}

// Rebuild 用新词表重建并原子切换。
//
// 构建失败时返回 error，旧匹配器继续生效（不切换）—— 后台保存一份坏词表不该把
// 线上正在用的词表一起弄丢。
func (e *Engine) Rebuild(words []Word, whitelist []string) error {
	if e == nil {
		return fmt.Errorf("contentfilter: 引擎未初始化（需先用 NewEngine 构建）")
	}
	m, err := buildMatcher(words, whitelist)
	if err != nil {
		return err
	}
	e.state.Store(m)
	return nil
}

// WordCount 返回当前生效的词条数（启动日志与后台概览用）。
func (e *Engine) WordCount() int {
	m := e.current()
	if m == nil {
		return 0
	}
	return len(m.meta)
}

// Filter 过滤一段文本：返回等长替换后的结果与命中明细。
//
// 空引擎（未初始化）按"原样返回、无命中"处理：这条路径只可能出现在装配遗漏时，
// 让它 panic 会把一次配置错误放大成整站不可用。
func (e *Engine) Filter(text string) Result {
	m := e.current()
	if m == nil || text == "" {
		return Result{Masked: text}
	}
	return m.apply(text)
}

// current 取当前匹配器；未初始化时返回 nil。
func (e *Engine) current() *matcher {
	if e == nil {
		return nil
	}
	m, _ := e.state.Load().(*matcher)
	return m
}

// buildMatcher 构建一份新的匹配器。
func buildMatcher(words []Word, whitelist []string) (*matcher, error) {
	keys := make([]string, 0, len(words))
	meta := make(map[string]Word, len(words))
	for _, entry := range words {
		word := strings.TrimSpace(entry.Word)
		key := compact(word)
		// 单字词不入表：中文单字（"毒""赌"）会命中大量正常文本，误报压过收益。
		if utf8.RuneCountInString(key) < MinWordRunes {
			continue
		}
		tier, ok := ParseTier(string(entry.Tier))
		if !ok {
			return nil, fmt.Errorf("contentfilter: 词条 %q 的档位 %q 不认识", word, entry.Tier)
		}
		category, ok := ParseCategory(string(entry.Category))
		if !ok {
			return nil, fmt.Errorf("contentfilter: 词条 %q 的类目 %q 不认识", word, entry.Category)
		}
		if _, dup := meta[key]; dup {
			continue
		}
		meta[key] = Word{Word: word, Tier: tier, Category: category}
		keys = append(keys, key)
	}

	whiteKeys := make([]string, 0, len(whitelist))
	seenWhite := make(map[string]struct{}, len(whitelist))
	for _, entry := range whitelist {
		key := compact(entry)
		if key == "" {
			continue
		}
		if _, dup := seenWhite[key]; dup {
			continue
		}
		seenWhite[key] = struct{}{}
		whiteKeys = append(whiteKeys, key)
	}

	// 一次性建树：trie.AddKeywords 每调一次都会重算整棵树的失败指针与关键词继承，
	// 逐条加两万词会退化成 O(n²)。
	return &matcher{
		dict:  trie.New(keys...),
		white: trie.New(whiteKeys...),
		meta:  meta,
	}, nil
}
