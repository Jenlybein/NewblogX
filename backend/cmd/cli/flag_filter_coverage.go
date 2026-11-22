package cli

import (
	"fmt"

	"myblogx/internal/platform/config"
	"myblogx/internal/platform/contentfilter"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlagFilterCoverage 打印当前库里的词表覆盖情况：总量 + 七类分布 + 缺失的必需类目。
//
// 为什么需要这条命令：词表是**运行时配置**，线上到底有哪些类目的词，只能看库。
// 而按 ADR-0032 决策 8，涉政与暴恐词只进数据库、不进仓库，所以"仓库里全绿"
// 完全不代表线上词库完整 —— 2026-11-22 生产实测就是 politics 0 / violence 0。
//
// 输出**只有类目名与条数**，不打印任何词条：词表是敏感数据，
// 这条命令的输出会进运维记录与工单。
//
// 不因缺类目返回 error：它是只读体检，返回值非 0 会让 `set -e` 的脚本在
// "体检发现问题"时中断后续步骤；是否报警由调用方按输出判断（启动日志与后台
// 页面各自有红灯）。
func FlagFilterCoverage(db *gorm.DB, logger *logrus.Logger, configFile string) error {
	if db == nil {
		return fmt.Errorf("词表覆盖体检失败: 数据库未初始化")
	}

	// 与导入命令同一口径读默认值：库里的词表段缺字段时由 site_default_settings.yaml 补齐。
	defaults, defaultsErr := config.LoadDefaults(configFile)
	if defaultsErr != nil && logger != nil {
		logger.Warnf("读取站点默认配置失败，词表体检将只用库里那份运行时配置: %v", defaultsErr)
	}

	svc := config.NewRuntimeConfigService(defaults, logger, db)
	if err := svc.InitRuntimeConfig(); err != nil {
		return fmt.Errorf("词表覆盖体检失败: 读取运行时配置: %w", err)
	}

	contentFilter := svc.GetRuntimeContentFilter()
	words := make([]contentfilter.Word, 0, len(contentFilter.Words))
	for _, entry := range contentFilter.Words {
		words = append(words, contentfilter.Word{
			Word:     entry.Word,
			Tier:     contentfilter.Tier(entry.Tier),
			Category: contentfilter.Category(entry.Category),
		})
	}

	counts := contentfilter.CategoryCounts(words)
	report := fmt.Sprintf("词表覆盖：总词条 %d 条；类目分布 %s；白名单 %d 条",
		len(words),
		contentfilter.FormatCategoryCounts(counts),
		len(contentFilter.Whitelist),
	)

	if logger != nil {
		logger.Infof("%s", report)
	}
	fmt.Println(report)

	if hint := contentfilter.CategoryCoverageHint(contentfilter.MissingRequiredCategories(words)); hint != "" {
		if logger != nil {
			logger.Warnf("%s", hint)
		}
		fmt.Println("⚠ " + hint)
		return nil
	}

	msg := "词表覆盖体检通过：七个必需类目均非空"
	if logger != nil {
		logger.Infof("%s", msg)
	}
	fmt.Println(msg)
	return nil
}
