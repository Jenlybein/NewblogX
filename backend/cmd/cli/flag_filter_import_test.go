package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// fakeFilterStore 是内存版词表存储：断言"dry-run 不写"与"apply 幂等"不需要真库。
type fakeFilterStore struct {
	current   conf.ContentFilter
	writes    int
	failWrite bool
}

// ContentFilter 实现 filterWordStore。
func (f *fakeFilterStore) ContentFilter() conf.ContentFilter { return f.current }

// UpdateContentFilter 实现 filterWordStore。
func (f *fakeFilterStore) UpdateContentFilter(contentFilter conf.ContentFilter) error {
	if f.failWrite {
		return errors.New("磁盘满了")
	}
	f.current = contentFilter
	f.writes++
	return nil
}

// writeTempWordlist 写一份临时词表文件（导入命令只读它，不读目录）。
func writeTempWordlist(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "words.tsv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时词表失败: %v", err)
	}
	return path
}

// ── 解析 ──────────────────────────────────────────────────────────

// TSV 与 NDJSON 都要认；空行与 # 注释跳过；BOM 不影响第一行。
func TestParseFilterImportFileAcceptsTSVAndNDJSON(t *testing.T) {
	path := writeTempWordlist(t, "\ufeff冰毒\tblock\tdrugs\n"+
		"# 这是注释\n"+
		"\n"+
		"{\"word\":\"成人视频\",\"tier\":\"watch\",\"category\":\"porn\"}\n")

	entries, lines, skipped, err := parseFilterImportFile(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("应解析出 2 条，得到 %d 条: %+v", len(entries), entries)
	}
	if lines != 4 || skipped != 2 {
		t.Fatalf("行数统计不对: lines=%d skipped=%d", lines, skipped)
	}
	if entries[0].Word != "冰毒" || entries[0].Tier != "block" || entries[0].Category != "drugs" {
		t.Fatalf("BOM 未剥离或 TSV 解析错误: %+v", entries[0])
	}
	if entries[1].Word != "成人视频" || entries[1].Category != "porn" {
		t.Fatalf("NDJSON 解析错误: %+v", entries[1])
	}
}

// 列数不对、JSON 坏掉都要报出行号，不能静默跳过。
func TestParseFilterImportFileReportsLineNumbers(t *testing.T) {
	path := writeTempWordlist(t, "冰毒\tblock\n")
	if _, _, _, err := parseFilterImportFile(path); err == nil || !strings.Contains(err.Error(), "第 1 行") {
		t.Fatalf("两列 TSV 应报出第 1 行的错误，得到 %v", err)
	}

	path = writeTempWordlist(t, "冰毒\tblock\tdrugs\n{\"word\":\n")
	if _, _, _, err := parseFilterImportFile(path); err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("坏 JSON 应报出第 2 行的错误，得到 %v", err)
	}

	if _, _, _, err := parseFilterImportFile(filepath.Join(t.TempDir(), "不存在.tsv")); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
}

// ── 合并 ──────────────────────────────────────────────────────────

// dry-run 只报告，一次写库都不发生。
func TestImportContentFilterDryRunDoesNotWrite(t *testing.T) {
	store := &fakeFilterStore{}
	entries := []conf.ContentFilterWord{
		{Word: "冰毒", Tier: "block", Category: "drugs"},
		{Word: "成人视频", Tier: "watch", Category: "porn"},
	}

	report, err := importContentFilter(store, entries, false)
	if err != nil {
		t.Fatalf("预演失败: %v", err)
	}
	if store.writes != 0 {
		t.Fatalf("dry-run 不应写库，实际写了 %d 次", store.writes)
	}
	if report.Added != 2 || report.Total != 2 || report.Valid != 2 {
		t.Fatalf("预演统计不对: %+v", report)
	}
	if report.Categories["drugs"] != 1 || report.Categories["porn"] != 1 {
		t.Fatalf("类目分布不对: %+v", report.Categories)
	}
	if report.Tiers["block"] != 1 || report.Tiers["watch"] != 1 {
		t.Fatalf("档位分布不对: %+v", report.Tiers)
	}
}

// apply 之后再导一次：新增 0 条、总数不变、词条不重复（幂等）。
func TestImportContentFilterApplyIsIdempotent(t *testing.T) {
	store := &fakeFilterStore{}
	entries := []conf.ContentFilterWord{
		{Word: "冰毒", Tier: "block", Category: "drugs"},
		{Word: "冰毒", Tier: "watch", Category: "fraud"}, // 文件内重复：保留首次出现
		{Word: "成人视频", Tier: "watch", Category: "porn"},
	}

	first, err := importContentFilter(store, entries, true)
	if err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	if store.writes != 1 {
		t.Fatalf("apply 应写库 1 次，实际 %d 次", store.writes)
	}
	if first.Added != 2 || first.DuplicateInFile != 1 || first.Total != 2 {
		t.Fatalf("首次导入统计不对: %+v", first)
	}
	if got := store.current.Words[0]; got.Word != "冰毒" || got.Tier != "block" {
		t.Fatalf("文件内重复应保留首次出现的档位，得到 %+v", got)
	}

	second, err := importContentFilter(store, entries, true)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if second.Added != 0 || second.Existing != 2 || second.Total != 2 {
		t.Fatalf("重复导入应全部跳过: %+v", second)
	}
	if store.writes != 1 {
		t.Fatalf("没有新增时不应再写库，实际写了 %d 次", store.writes)
	}
	if len(store.current.Words) != 2 {
		t.Fatalf("词表不应出现重复词条: %+v", store.current.Words)
	}
}

// 同词保留已有项：库里的档位/类目不会被导入文件覆盖，白名单也不动。
func TestImportContentFilterKeepsExistingEntries(t *testing.T) {
	store := &fakeFilterStore{current: conf.ContentFilter{
		Words:     []conf.ContentFilterWord{{Word: "刷单", Tier: "watch", Category: "fraud"}},
		Whitelist: []string{"测试"},
	}}

	report, err := importContentFilter(store, []conf.ContentFilterWord{
		{Word: "刷单", Tier: "block", Category: "drugs"},
		{Word: "冰毒", Tier: "block", Category: "drugs"},
	}, true)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Existing != 1 || report.Added != 1 {
		t.Fatalf("统计不对: %+v", report)
	}
	if len(store.current.Words) != 2 {
		t.Fatalf("应保留原有 1 条并新增 1 条，得到 %+v", store.current.Words)
	}
	existing := store.current.Words[0]
	if existing.Word != "刷单" || existing.Tier != "watch" || existing.Category != "fraud" {
		t.Fatalf("已存在的词条不应被覆盖: %+v", existing)
	}
	if len(store.current.Whitelist) != 1 || store.current.Whitelist[0] != "测试" {
		t.Fatalf("白名单不应被导入改动: %+v", store.current.Whitelist)
	}
}

// 单字词、未知档位、未知类目一律拒绝：整批不写，避免写半份词表。
func TestImportContentFilterRejectsInvalidEntries(t *testing.T) {
	cases := map[string][]conf.ContentFilterWord{
		"单字词":  {{Word: "毒", Tier: "block", Category: "drugs"}},
		"未知档位": {{Word: "冰毒", Tier: "blocked", Category: "drugs"}},
		"未知类目": {{Word: "冰毒", Tier: "block", Category: "weapons"}},
	}
	for name, entries := range cases {
		store := &fakeFilterStore{}
		if _, err := importContentFilter(store, entries, true); err == nil {
			t.Fatalf("%s 应当被拒绝", name)
		}
		if store.writes != 0 {
			t.Fatalf("%s 被拒绝后不应写库，实际写了 %d 次", name, store.writes)
		}
	}
}

// 写库失败要把错误带出来（不能假装成功）。
func TestImportContentFilterSurfacesWriteError(t *testing.T) {
	store := &fakeFilterStore{failWrite: true}
	_, err := importContentFilter(store, []conf.ContentFilterWord{
		{Word: "冰毒", Tier: "block", Category: "drugs"},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "写入词表失败") {
		t.Fatalf("写库失败应返回错误，得到 %v", err)
	}
}

// ── 命令入口 ──────────────────────────────────────────────────────

// 缺文件、缺库都要直接报错，而不是"看起来跑过了"。
func TestFlagFilterImportRequiresFileAndDB(t *testing.T) {
	if err := FlagFilterImport(nil, nil, "", "", true); err == nil {
		t.Fatal("未指定文件时应报错")
	}
	if err := FlagFilterImport(nil, nil, "", "words.tsv", true); err == nil {
		t.Fatal("数据库未初始化时应报错")
	}
}

// Run 的分支接线：只给 -filter-import-apply 不给文件要报错（不静默继续启动）。
func TestRunFilterImportRequiresFileName(t *testing.T) {
	_, err := Run(&FlagOptions{FilterImportApply: true}, Options{})
	if err == nil || !strings.Contains(err.Error(), "-filter-import") {
		t.Fatalf("缺 -filter-import 时应报错，得到 %v", err)
	}
}

// 端到端（sqlite）：预演不动库，apply 写进去，再 apply 一次内容不变。
//
// 用真数据库而不是 fake store，是因为"写库之后读回来还是同一份"这条只有整条链路
// 才能证明：runtime 配置段是 JSON 落库 + 缓存刷新的组合。
func TestFlagFilterImportEndToEnd(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})
	path := writeTempWordlist(t, "冰毒\tblock\tdrugs\n成人视频\twatch\tporn\n")

	readPayload := func() string {
		var row schema.RuntimeConfigRow
		if err := db.Take(&row, "name = ?", "content_filter").Error; err != nil {
			t.Fatalf("读取 content_filter 段失败: %v", err)
		}
		return row.Payload
	}

	if err := FlagFilterImport(db, nil, "", path, false); err != nil {
		t.Fatalf("预演失败: %v", err)
	}
	if strings.Contains(readPayload(), "冰毒") {
		t.Fatal("dry-run 不应写入词条")
	}

	if err := FlagFilterImport(db, nil, "", path, true); err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	applied := readPayload()
	if !strings.Contains(applied, "冰毒") || !strings.Contains(applied, "成人视频") {
		t.Fatalf("apply 后应写入两条词条，实际 %s", applied)
	}

	if err := FlagFilterImport(db, nil, "", path, true); err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if again := readPayload(); again != applied {
		t.Fatalf("重复导入应幂等，payload 变了:\n第一次 %s\n第二次 %s", applied, again)
	}
}
