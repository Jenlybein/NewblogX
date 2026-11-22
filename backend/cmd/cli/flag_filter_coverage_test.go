package cli

import (
	"testing"

	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
)

// readContentFilterPayload 读 content_filter 段的原始 JSON（只用来比对"体检不改库"）。
func readContentFilterPayload(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var row schema.RuntimeConfigRow
	if err := db.Take(&row, "name = ?", "content_filter").Error; err != nil {
		t.Fatalf("读取 content_filter 段失败: %v", err)
	}
	return row.Payload
}

// 词表覆盖体检（ADR-0032 决策 8 的运维补丁）：
//   - 只读：跑完不改库里任何东西；
//   - 缺 politics / violence 时报出来（生产实测就是这两类为 0）；
//   - 补齐后判通过。
//
// 断言的是"命令能不能把缺口说出来"，不是它内部用哪个查询。

func TestFlagFilterCoverageReportsMissingCategories(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})

	// 先导入一份"只有五类"的词表：正是生产当时的形状。
	path := writeTempWordlist(t, "冰毒\tblock\tdrugs\n"+"成人视频\twatch\tporn\n"+"刷单\twatch\tfraud\n")
	if err := FlagFilterImport(db, nil, "../../config/settings.yaml", path, true); err != nil {
		t.Fatalf("导入基础词表失败: %v", err)
	}

	before := readContentFilterPayload(t, db)

	// 体检必须跑得通，且不改库（只读命令改了数据是最难查的 bug）。
	if err := FlagFilterCoverage(db, nil, "../../config/settings.yaml"); err != nil {
		t.Fatalf("词表覆盖体检失败: %v", err)
	}
	if after := readContentFilterPayload(t, db); after != before {
		t.Fatal("体检不该修改词表段")
	}
}

func TestFlagFilterCoveragePassesWhenComplete(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})

	// 七类齐全（每类一条足够）：体检应判通过。
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
		t.Fatalf("七类齐全时体检应通过，实际报错: %v", err)
	}
}
