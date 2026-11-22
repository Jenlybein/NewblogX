package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"myblogx/conf"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/contentfilter"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 本文件是"初始词表导入"（#395）：把 scripts/content-filter/build-wordlist.mjs 生成的
// TSV/NDJSON 灌进 content_filter 段（第 11 段，ADR-0032）。
//
// 三条设计：
//   - **默认 dry-run**：不带 -filter-import-apply 只报告不写库（与 -t reconcile、
//     -t imagegc 的口径一致）。词表是线上的过滤依据，一次误导入要靠回滚才能恢复。
//   - **幂等**：同一个文件重复导入不会产生重复词条 —— 已存在的词（按小写比较）
//     保留库里的写法与档位，导入文件只是给它补上缺的词。
//   - **校验与去重口径复用站点保存用例**（contentfilter.NormalizeWords）：
//     导入能写进去的词，后台保存一定也能接受；反之亦然。
//
// 词条本身**不进日志**：日志只记数量与类目分布（ADR-0032 安全约束）。

// filterWordStore 是导入命令需要的词表读写能力。
//
// 抽成接口是为了让"dry-run 不写 / apply 幂等"这两条能被测试直接断言，
// 不必为此在测试里拉一个真实 MySQL。
type filterWordStore interface {
	// ContentFilter 返回当前词表段。
	ContentFilter() conf.ContentFilter
	// UpdateContentFilter 覆盖词表段。
	UpdateContentFilter(contentFilter conf.ContentFilter) error
}

// runtimeFilterWordStore 把运行时配置服务包成 filterWordStore。
type runtimeFilterWordStore struct {
	svc *config.RuntimeConfigService
}

// ContentFilter 实现 filterWordStore。
func (s runtimeFilterWordStore) ContentFilter() conf.ContentFilter {
	return s.svc.GetRuntimeContentFilter()
}

// UpdateContentFilter 实现 filterWordStore。
func (s runtimeFilterWordStore) UpdateContentFilter(contentFilter conf.ContentFilter) error {
	return s.svc.UpdateContentFilter(contentFilter)
}

// filterImportReport 是一次导入的结果，用来打印与写日志。
type filterImportReport struct {
	// File 是导入文件名。
	File string
	// Lines 是文件总行数（含空行与注释）。
	Lines int
	// Entries 是解析出来的词条数（未校验）。
	Entries int
	// SkippedLines 是被忽略的空行 / 注释行。
	SkippedLines int
	// Valid 是通过校验的词条数。
	Valid int
	// DuplicateInFile 是文件内部重复（已被校验去重）的词条数。
	DuplicateInFile int
	// Existing 是库里已有、因此跳过的词条数。
	Existing int
	// Added 是新增的词条数。
	Added int
	// Total 是写入（或 dry-run 预计）的词表总条数。
	Total int
	// Categories 是导入后词表的类目分布。
	Categories map[string]int
	// Tiers 是导入后词表的档位分布。
	Tiers map[string]int
}

// FlagFilterImport 从文件导入初始词表，按 apply 决定是否落库。
//
// configFile 用来定位 site_default_settings.yaml：词表段里缺字段时由那里的默认值
// 补齐（与站点配置的合并语义一致），同时让"首次上线本段"的旧词表合并（#394 的启动
// 迁移）在这条命令上也能发生 —— 否则运维先跑导入、再启服务，两边的合并顺序就反了。
func FlagFilterImport(db *gorm.DB, logger *logrus.Logger, configFile, file string, apply bool) error {
	if strings.TrimSpace(file) == "" {
		return fmt.Errorf("初始词表导入失败: 未指定词表文件（用 -filter-import <file>）")
	}
	if db == nil {
		return fmt.Errorf("初始词表导入失败: 数据库未初始化")
	}

	svc, err := runtimeConfigServiceForCommand(db, logger, configFile, "初始词表导入", false)
	if err != nil {
		return err
	}

	entries, lines, skipped, err := parseFilterImportFile(file)
	if err != nil {
		return fmt.Errorf("初始词表导入失败: %w", err)
	}

	report, err := importContentFilter(runtimeFilterWordStore{svc: svc}, entries, apply)
	if err != nil {
		return fmt.Errorf("初始词表导入失败: %w", err)
	}
	report.File = file
	report.Lines = lines
	report.SkippedLines = skipped

	printFilterImportReport(report, apply)
	if apply && report.Added > 0 {
		// 常驻进程各自持有自己那份运行时配置缓存：这条命令写的是库，不会顺带
		// 通知已在跑的服务。不说清楚，运维会以为"导完了但线上没反应"是 bug。
		fmt.Println("\n提示: 已在运行的服务需要重启（或在后台「内容过滤」保存一次）才会用上新词表。")
	}
	if logger != nil {
		// 只记数量，不记词条：日志会被采集，词表不该出现在里面。
		logger.Infof("初始词表导入完成: file=%s apply=%t valid=%d added=%d existing=%d total=%d",
			file, apply, report.Valid, report.Added, report.Existing, report.Total)
	}
	return nil
}

// importContentFilter 执行"校验 → 与现有词表合并 → 按需写库"。
//
// 与 store 解耦，便于测试用内存实现断言"dry-run 不写、apply 幂等"。
func importContentFilter(store filterWordStore, entries []conf.ContentFilterWord, apply bool) (filterImportReport, error) {
	report := filterImportReport{
		Entries:    len(entries),
		Categories: map[string]int{},
		Tiers:      map[string]int{},
	}
	if apply && store == nil {
		return report, fmt.Errorf("未提供词表存储")
	}

	// 先按站点保存用例的口径校验文件内容：坏数据在写库前就要炸，
	// 而不是等过滤引擎构建失败（那时表现成整站启动不了）。
	normalized, err := contentfilter.NormalizeWords(toFilterWords(entries))
	if err != nil {
		return report, fmt.Errorf("词表文件不合法: %w", err)
	}
	report.Valid = len(normalized)
	report.DuplicateInFile = countNonEmptyWords(entries) - len(normalized)

	current := conf.ContentFilter{}
	if store != nil {
		current = store.ContentFilter()
	}
	merged := make([]conf.ContentFilterWord, 0, len(current.Words)+len(normalized))
	seen := make(map[string]struct{}, len(current.Words)+len(normalized))
	for _, word := range current.Words {
		key := strings.ToLower(strings.TrimSpace(word.Word))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, word)
	}
	for _, word := range normalized {
		key := strings.ToLower(word.Word)
		if _, dup := seen[key]; dup {
			// 同词保留已有项：导入文件不可能比人工维护的档位/类目更权威。
			report.Existing++
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, conf.ContentFilterWord{
			Word:     word.Word,
			Tier:     string(word.Tier),
			Category: string(word.Category),
		})
		report.Added++
	}

	report.Total = len(merged)
	for _, word := range merged {
		report.Tiers[word.Tier]++
		report.Categories[word.Category]++
	}

	if !apply || report.Added == 0 {
		return report, nil
	}
	if err := store.UpdateContentFilter(conf.ContentFilter{
		Words:     merged,
		Whitelist: current.Whitelist,
	}); err != nil {
		return report, fmt.Errorf("写入词表失败: %w", err)
	}
	return report, nil
}

// parseFilterImportFile 读取导入文件，返回词条、总行数与被忽略的行数。
//
// 两种格式都认：TSV（`词<TAB>档位<TAB>类目`）与 NDJSON（每行一个
// `{"word","tier","category"}`）。空行与 `#` 开头的注释行跳过。
func parseFilterImportFile(path string) ([]conf.ContentFilterWord, int, int, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("打开词表文件失败: %w", err)
	}
	defer func() { _ = handle.Close() }()

	const maxLineBytes = 1 << 20
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	entries := make([]conf.ContentFilterWord, 0, 4096)
	lines, skipped := 0, 0
	for scanner.Scan() {
		lines++
		line := strings.TrimPrefix(scanner.Text(), "\ufeff")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			skipped++
			continue
		}
		entry, parseErr := parseFilterImportLine(trimmed)
		if parseErr != nil {
			return nil, lines, skipped, fmt.Errorf("第 %d 行: %w", lines, parseErr)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, lines, skipped, fmt.Errorf("读取词表文件失败: %w", err)
	}
	return entries, lines, skipped, nil
}

// parseFilterImportLine 解析一行（TSV 或 NDJSON）。
func parseFilterImportLine(line string) (conf.ContentFilterWord, error) {
	if strings.HasPrefix(line, "{") {
		var payload struct {
			Word     string `json:"word"`
			Tier     string `json:"tier"`
			Category string `json:"category"`
		}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return conf.ContentFilterWord{}, fmt.Errorf("不是合法的 JSON 词条: %w", err)
		}
		if strings.TrimSpace(payload.Word) == "" {
			return conf.ContentFilterWord{}, fmt.Errorf("JSON 词条缺少 word")
		}
		return conf.ContentFilterWord{
			Word:     payload.Word,
			Tier:     strings.TrimSpace(payload.Tier),
			Category: strings.TrimSpace(payload.Category),
		}, nil
	}

	fields := strings.Split(line, "\t")
	if len(fields) != 3 {
		return conf.ContentFilterWord{}, fmt.Errorf(
			"TSV 需要 3 列（词<TAB>档位<TAB>类目），实际 %d 列", len(fields))
	}
	return conf.ContentFilterWord{
		Word:     strings.TrimSpace(fields[0]),
		Tier:     strings.TrimSpace(fields[1]),
		Category: strings.TrimSpace(fields[2]),
	}, nil
}

// toFilterWords 把配置词条转成引擎包的类型（校验逻辑在 contentfilter）。
func toFilterWords(entries []conf.ContentFilterWord) []contentfilter.Word {
	words := make([]contentfilter.Word, 0, len(entries))
	for _, entry := range entries {
		words = append(words, contentfilter.Word{
			Word:     entry.Word,
			Tier:     contentfilter.Tier(entry.Tier),
			Category: contentfilter.Category(entry.Category),
		})
	}
	return words
}

// countNonEmptyWords 数一下非空词条（用于算"文件内重复"）。
func countNonEmptyWords(entries []conf.ContentFilterWord) int {
	count := 0
	for _, entry := range entries {
		if strings.TrimSpace(entry.Word) != "" {
			count++
		}
	}
	return count
}

// printFilterImportReport 打印导入报告。
func printFilterImportReport(report filterImportReport, apply bool) {
	mode := "dry-run，未写库"
	if apply {
		mode = "已写入"
	}
	fmt.Printf("── 初始词表导入（%s）──\n", mode)
	fmt.Printf("文件:         %s\n", report.File)
	fmt.Printf("读入:         %d 行（空行/注释 %d 行）\n", report.Lines, report.SkippedLines)
	fmt.Printf("有效词条:     %d 条（文件内重复 %d 条）\n", report.Valid, report.DuplicateInFile)
	fmt.Printf("跳过已有:     %d 条\n", report.Existing)
	fmt.Printf("新增:         %d 条\n", report.Added)
	fmt.Printf("词表总数:     %d 条\n", report.Total)
	fmt.Printf("档位:         block %d / watch %d\n", report.Tiers[string(contentfilter.TierBlock)], report.Tiers[string(contentfilter.TierWatch)])
	// 类目顺序只认 contentfilter.AllCategories：这里再手列一遍，加类目时就会漏。
	fmt.Printf("类目:         ")
	for _, category := range contentfilter.AllCategories {
		fmt.Printf("%s %d  ", category, report.Categories[string(category)])
	}
	fmt.Println()
	if !apply {
		fmt.Println("\n这是预演：词表没有任何改动。带 -filter-import-apply 才会写入。")
	} else if report.Added == 0 {
		fmt.Println("\n没有新增词条：词表已是目标状态（重复执行安全）。")
	}
}
