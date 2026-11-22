package cli

import (
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
)

// 词表覆盖体检（ADR-0032 决策 8 的运维补丁）：
//   - 只读：跑完不改库里任何东西（**包括缺词表段时也不补默认行**）；
//   - 缺 politics / violence 时必须把缺口说出来，而不是静默通过；
//   - 输出只含类目名与数字，不含词条。

func readContentFilterPayload(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var row schema.RuntimeConfigRow
	if err := db.Take(&row, "name = ?", "content_filter").Error; err != nil {
		t.Fatalf("读取 content_filter 段失败: %v", err)
	}
	return row.Payload
}

// contentFilterWith 造一份词表段（只给必需的两类留空或不留空）。
func contentFilterWith(entries [][3]string) conf.ContentFilter {
	words := make([]conf.ContentFilterWord, 0, len(entries))
	for _, entry := range entries {
		words = append(words, conf.ContentFilterWord{Word: entry[0], Tier: entry[1], Category: entry[2]})
	}
	return conf.ContentFilter{Words: words}
}

// TestCoverageReportLinesSaysWhatIsMissing 钉住"体检要把缺口说出来"。
//
// 上一版的用例只断言"不报错、不改库"，名字里承诺的"报告缺类目"其实没测——
// 于是这条命令就算哪天不再报警，测试也照样绿。
func TestCoverageReportLinesSaysWhatIsMissing(t *testing.T) {
	t.Parallel()

	// 生产当时的形状：五类有词、涉政与暴恐为空。
	report := coverageReportLines(contentFilterWith([][3]string{
		{"冰毒", "block", "drugs"},
		{"成人视频", "watch", "porn"},
		{"刷单返利", "block", "fraud"},
	}))
	joined := strings.Join(report, "\n")
	if !strings.Contains(joined, "⚠") {
		t.Fatalf("缺类目时必须给 ⚠ 行，实际报告：%s", joined)
	}
	for _, must := range []string{"politics", "violence", "build-wordlist", "-filter-import"} {
		if !strings.Contains(joined, must) {
			t.Fatalf("缺口提示里应包含 %q，实际：%s", must, joined)
		}
	}
	// 类目分布要能看出"哪几类是 0"。
	if !strings.Contains(joined, "politics=0") || !strings.Contains(joined, "violence=0") {
		t.Fatalf("报告应显示 politics=0 violence=0，实际：%s", joined)
	}
}

// TestCoverageReportLinesPassesWhenComplete 钉住"补齐后不再报警"。
func TestCoverageReportLinesPassesWhenComplete(t *testing.T) {
	t.Parallel()

	report := coverageReportLines(contentFilterWith([][3]string{
		{"非法集会", "block", "politics"},
		{"炸药配方", "block", "violence"},
	}))
	joined := strings.Join(report, "\n")
	if strings.Contains(joined, "⚠") {
		t.Fatalf("两类都有词时不该报警，实际：%s", joined)
	}
	// 措辞只能说"必需类目"，不能说成七个类目都验过（实际只校验两类）。
	if !strings.Contains(joined, "必需类目均非空") || !strings.Contains(joined, "politics, violence") {
		t.Fatalf("通过语气应点名必需类目，实际：%s", joined)
	}
	if strings.Contains(joined, "七个") {
		t.Fatalf("不该声称校验了七个类目（只校验 RequiredCategories），实际：%s", joined)
	}
}

// TestFlagFilterCoverageReportsMissingCategories 端到端：缺类目能报、且不改库。
func TestFlagFilterCoverageReportsMissingCategories(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})

	// 先导入一份"只有五类"的词表：正是生产当时的形状。
	path := writeTempWordlist(t, "冰毒\tblock\tdrugs\n"+"成人视频\twatch\tporn\n"+"刷单\twatch\tfraud\n")
	if err := FlagFilterImport(db, nil, "../../config/settings.yaml", path, true); err != nil {
		t.Fatalf("导入基础词表失败: %v", err)
	}

	before := readContentFilterPayload(t, db)
	if err := FlagFilterCoverage(db, nil, "../../config/settings.yaml"); err != nil {
		t.Fatalf("词表覆盖体检失败: %v", err)
	}
	if after := readContentFilterPayload(t, db); after != before {
		t.Fatal("体检不该修改词表段")
	}
}

// TestFlagFilterCoverageIsReadOnlyOnEmptyDatabase 钉住"缺段时也不写库"。
//
// InitRuntimeConfig 会 seedMissing 补默认行；体检要是用它，第一次跑就把库改了，
// 而运维会以为"体检是只读的"。
func TestFlagFilterCoverageIsReadOnlyOnEmptyDatabase(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})

	if err := FlagFilterCoverage(db, nil, "../../config/settings.yaml"); err != nil {
		t.Fatalf("空库体检应能跑通（按默认值报告缺口），实际报错: %v", err)
	}

	var rows int64
	if err := db.Model(&schema.RuntimeConfigRow{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计运行时配置行失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("体检不该补默认行（InitRuntimeConfig 会），实际写了 %d 行", rows)
	}
}

// TestFlagFilterCoveragePassesWhenComplete 端到端：七类齐全时判通过。
func TestFlagFilterCoveragePassesWhenComplete(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})

	path := writeTempWordlist(t, "冰毒\tblock\tdrugs\n"+
		"博彩网站\tblock\tgambling\n"+
		"刷单返利\tblock\tfraud\n"+
		"成人视频\twatch\tporn\n"+
		"办假证\tblock\tfake_docs\n"+
		"非法集会\tblock\tpolitics\n"+
		"炸药配方\tblock\tviolence\n")
	if err := FlagFilterImport(db, nil, "../../config/settings.yaml", path, true); err != nil {
		t.Fatalf("导入完整词表失败: %v", err)
	}

	if err := FlagFilterCoverage(db, nil, "../../config/settings.yaml"); err != nil {
		t.Fatalf("两类必需类目都有词时体检应通过，实际报错: %v", err)
	}
}
