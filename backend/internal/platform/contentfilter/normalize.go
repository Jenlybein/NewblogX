package contentfilter

import (
	"unicode"
)

// zeroWidthSentinel 是零宽字符归一化后的占位符。
//
// 用非字符 U+FFFF 而不是直接删掉：删掉会让 rune 下标整体左移，命中位置就再也
// 映射不回原文了（"冰\u200b毒"删掉零宽后长度从 3 变 2，替换必然错位）。换成哨兵
// 后长度不变，再由影子串把它当分隔符压掉 —— 匹配能跨过它，替换仍按原位覆盖它。
const zeroWidthSentinel = '\uFFFF'

// foldRune 做 1:1 归一化：全角→半角、拉丁大写→小写、零宽字符→哨兵。
//
// 只做"一个 rune 换一个 rune"的变换：整段 NFKC 会改变 rune 数量（例如 ㍿ → 株式会社），
// 命中区间就无法映射回原文，替换一定错位。所以这里逐 rune 折叠，不做组合展开。
func foldRune(r rune) rune {
	switch {
	// 全角 ASCII（U+FF01–U+FF5E）与半角 ASCII 一一对应。
	case r >= 0xFF01 && r <= 0xFF5E:
		return r - 0xFEE0
	// 全角空格折叠为普通空格（同样 1:1）。
	case r == 0x3000:
		return ' '
	// 只折 ASCII 拉丁：其它文种的 ToLower 不保证 1:1，折了会破坏对齐。
	case r >= 'A' && r <= 'Z':
		return r + ('a' - 'A')
	case isZeroWidth(r):
		return zeroWidthSentinel
	}
	return r
}

// isZeroWidth 判断零宽/不可见字符（常见的"插零宽绕过"就用这几个）。
func isZeroWidth(r rune) bool {
	switch r {
	case 0x200B, // ZERO WIDTH SPACE
		0x200C, // ZERO WIDTH NON-JOINER
		0x200D, // ZERO WIDTH JOINER
		0x2060, // WORD JOINER
		0xFEFF, // ZERO WIDTH NO-BREAK SPACE（BOM）
		0x180E: // MONGOLIAN VOWEL SEPARATOR
		return true
	}
	return false
}

// isSeparator 判断一个（已归一化的）rune 是否在影子串里被压掉。
//
// 空白、标点、符号一律压掉：这是"插空格 / 插分隔符 / 插表情"绕过手段的对策。
// 代价是匹配会跨过标点（"办、假、证" 也命中"办假证"），这是有意的取舍 ——
// 关键词命中只影响替换与风险页，不做自动拒绝。
func isSeparator(r rune) bool {
	if r == zeroWidthSentinel {
		return true
	}
	if unicode.IsSpace(r) {
		return true
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// buildShadow 生成"压掉空白与分隔符"的影子串，并记录每个字符映射回原文的 rune 下标。
//
// shadow 里第 i 个字符来自原文（归一化之后）的第 shadowIndex[i] 个 rune。
func buildShadow(folded []rune) (shadow []rune, shadowIndex []int) {
	shadow = make([]rune, 0, len(folded))
	shadowIndex = make([]int, 0, len(folded))
	for i, r := range folded {
		if isSeparator(r) {
			continue
		}
		shadow = append(shadow, r)
		shadowIndex = append(shadowIndex, i)
	}
	return shadow, shadowIndex
}

// compact 把任意字符串压成影子串（词表与白名单走同一条归一化路径）。
//
// 词表必须在与正文完全相同的归一化空间里构建：否则"词表里的词"和"正文里的词"
// 各自被折叠成什么就取决于谁先被写，匹配结果会随写法漂移。
func compact(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		folded := foldRune(r)
		if isSeparator(folded) {
			continue
		}
		out = append(out, folded)
	}
	return string(out)
}
