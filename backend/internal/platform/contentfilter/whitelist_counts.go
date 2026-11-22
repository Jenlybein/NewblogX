package contentfilter

// WhitelistCounts 是白名单的三个口径：总数（去重后）、词级、短语级。
//
// 分开算的原因是它们曾经对不上：总数用合并去重后的长度、两个分项用原始长度，
// 于是"词 46 + 短语 4"加不出 50，运维只能猜哪个数是真的。这里一次算清，
// 体检输出与后台展示都读它 —— 同一份判据只有一个来源。
type WhitelistCounts struct {
	// Total 是合并去重后的条数（引擎实际拿到的那张表）。
	Total int
	// Words 是词级白名单去重后的条数。
	Words int
	// Phrases 是短语级白名单去重后的条数。
	Phrases int
	// Duplicates 是两张表之间重复（同一条填了两次）的条数，Words+Phrases-Duplicates == Total。
	Duplicates int
}

// CountWhitelist 统计白名单的三个口径。
func CountWhitelist(words, phrases []string) WhitelistCounts {
	normalizedWords := NormalizeWhitelist(words)
	normalizedPhrases := NormalizeWhitelist(phrases)
	total := len(MergeWhitelist(normalizedWords, normalizedPhrases))
	return WhitelistCounts{
		Total:      total,
		Words:      len(normalizedWords),
		Phrases:    len(normalizedPhrases),
		Duplicates: len(normalizedWords) + len(normalizedPhrases) - total,
	}
}
