package contentfilter

import (
	"sort"
	"strconv"
	"strings"
)

// RequiredCategories 是"一个能用的词库必须非空"的类目。
//
// 为什么只有这两类：其余五类在上游词库里天然有大量词条，缺了通常是"导入失败"，
// 一次导入报告就能看出来；而这两类按 ADR-0032 决策 8 **只进数据库、不进仓库**，
// 必须由部署侧单独生成并导入 —— 仓库里默认排除、CI 里也看不到，所以很容易
// 在"词库看起来有 3 万条"的情况下静默为空（2026-11-22 生产实测：
// politics 0 条、violence 0 条，而七个文本面当时对涉政内容完全不拦）。
//
// 因此这里把它们列成"必须非空"，由启动、保存词表、运维命令三处一起报警。
var RequiredCategories = []Category{CategoryPolitics, CategoryViolence}

// AllCategories 是七个类目的固定顺序（输出排序、日志、后台筛选都按它）。
//
// 放在这里而不是让各处自己列：类目一旦增删，只有一处要改。
var AllCategories = []Category{
	CategoryDrugs,
	CategoryGambling,
	CategoryFraud,
	CategoryPorn,
	CategoryFakeDocs,
	CategoryPolitics,
	CategoryViolence,
}

// CategoryCounts 统计每个类目的词条数（未出现的类目不补 0）。
func CategoryCounts(words []Word) map[Category]int {
	counts := make(map[Category]int, len(AllCategories))
	for _, word := range words {
		counts[word.Category]++
	}
	return counts
}

// MissingRequiredCategories 返回 RequiredCategories 里条数为 0 的类目，顺序固定。
//
// 只看条数、不读词条内容：调用方拿到的是类目名，可以安全地写进日志与接口响应。
func MissingRequiredCategories(words []Word) []Category {
	counts := CategoryCounts(words)
	missing := make([]Category, 0, len(RequiredCategories))
	for _, category := range RequiredCategories {
		if counts[category] == 0 {
			missing = append(missing, category)
		}
	}
	return missing
}

// FormatCategoryCounts 把类目分布格式化成稳定顺序的一行（日志与命令行共用）。
//
// 顺序按 CATEGORIES 而不是 map 遍历：日志要能肉眼对比两次输出，
// 顺序随机的话每次都得重新找一遍。
func FormatCategoryCounts(counts map[Category]int) string {
	parts := make([]string, 0, len(AllCategories))
	for _, category := range AllCategories {
		parts = append(parts, string(category)+"="+strconv.Itoa(counts[category]))
	}
	return strings.Join(parts, " ")
}

// CategoryCoverageHint 是"缺类目"时给运维的下一步动作（日志与后台提示共用一句）。
//
// 刻意写清楚了要去哪台机器做什么：这句话通常出现在凌晨的启动日志里，
// 读它的人不该还需要回来翻 ADR。
func CategoryCoverageHint(missing []Category) string {
	if len(missing) == 0 {
		return ""
	}
	names := make([]string, 0, len(missing))
	for _, category := range missing {
		names = append(names, string(category))
	}
	sort.Strings(names)
	return "缺少必需类目词条: " + strings.Join(names, ", ") +
		"（这两类只进数据库、不进仓库；在部署机生成后导入：" +
		"node scripts/content-filter/build-wordlist.mjs --include-all --out /tmp/words.tsv 然后 " +
		"server -f config/settings.yaml -filter-import /tmp/words.tsv -filter-import-apply）"
}
