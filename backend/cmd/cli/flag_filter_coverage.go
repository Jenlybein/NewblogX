package cli

import (
	"fmt"
	"strings"

	"myblogx/conf"
	"myblogx/internal/platform/contentfilter"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 词表覆盖体检（`-filter-coverage`）：只读、只打印类目名与条数。
//
// 为什么需要它：词表是**运行时配置**，线上到底有哪些类目的词，只能看库。而按
// ADR-0032 决策 8，涉政与暴恐词只进数据库、不进仓库，所以"仓库里全绿"完全不代表
// 线上词库完整 —— 2026-11-22 生产实测就是 politics 0 / violence 0，而六个文本面
// 对涉政内容完全不拦，没有任何环节报错。
//
// 三条硬约束（改这个文件前先读）：
//  1. **只读**：走 LoadRuntimeConfig，不用 InitRuntimeConfig（后者会补默认行 = 写库）；
//  2. **不打印词条**：词表是敏感数据，输出会进运维记录与工单，只允许出现类目名与数字；
//  3. **不因缺类目返回 error**：返回值非 0 会让 `set -e` 的脚本在"体检发现问题"时
//     中断后续步骤；是否报警由调用方看输出判断（启动日志与后台页面各自有红灯）。
func FlagFilterCoverage(db *gorm.DB, logger *logrus.Logger, configFile string) error {
	if db == nil {
		return fmt.Errorf("词表覆盖体检失败: 数据库未初始化")
	}

	// 与导入命令同一口径读默认值（缺字段由 site_default_settings.yaml 补齐），
	// 但**只读**：走 LoadRuntimeConfig，不补默认行。
	svc, err := runtimeConfigServiceForCommand(db, logger, configFile, "词表覆盖体检", true)
	if err != nil {
		return err
	}

	for _, line := range coverageReportLines(svc.GetRuntimeContentFilter()) {
		if logger != nil {
			// 告警行单独用 Warn 级打一次，方便按级别筛；其余走 Info。
			if strings.HasPrefix(line, "⚠") {
				logger.Warnf("%s", line)
			} else {
				logger.Infof("%s", line)
			}
		}
		fmt.Println(line)
	}
	return nil
}

// coverageReportLines 把词表段渲染成体检报告（纯函数，便于直接断言）。
//
// 措辞刻意只说"必需类目"（`RequiredCategories` 现在是 politics 与 violence 两类），
// 不写"七个类目都非空"—— 那样等于宣称验了七类，而实际只校验这两类。
func coverageReportLines(contentFilter conf.ContentFilter) []string {
	words := toFilterWords(contentFilter.Words)
	counts := contentfilter.CategoryCounts(words)
	lines := []string{
		fmt.Sprintf("词表覆盖：总词条 %d 条；类目分布 %s；白名单 %d 条（词 %d + 短语 %d）",
			len(words),
			contentfilter.FormatCategoryCounts(counts),
			len(contentfilter.MergeWhitelist(contentFilter.Whitelist, contentFilter.WhitelistPhrases)),
			len(contentFilter.Whitelist),
			len(contentFilter.WhitelistPhrases),
		),
	}

	missing := contentfilter.MissingRequiredCategories(words)
	if len(missing) > 0 {
		return append(lines, "⚠ "+contentfilter.CategoryCoverageHint(missing))
	}
	return append(lines, fmt.Sprintf("词表覆盖体检通过：必需类目均非空（%s）",
		categoryNames(contentfilter.RequiredCategories)))
}

// categoryNames 把类目列表渲染成 `politics, violence` 这样的短语。
func categoryNames(categories []contentfilter.Category) string {
	names := make([]string, 0, len(categories))
	for _, category := range categories {
		names = append(names, string(category))
	}
	return strings.Join(names, ", ")
}
