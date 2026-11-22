package cli

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// 本文件钉住"存量清洗按类目归因"这条误伤评估口径：
// 词表变大以后，"命中 176 处"这个总数无法判断该不该 apply ——
// 176 处来自涉政宽泛词和来自毒品垃圾词是完全不同的两件事。
//
// 两个约束（上一版都踩过）：
//  1. 报告只含类目名与数字，不含词条（词表是敏感数据）；
//  2. 口径是**命中实体条数**，不是替换处数 —— 处数是区间并集，硬凑相等只会让人
//     以为其中一个是错的；标签必须说实话。

// TestFilterSweepCategoryLine 钉住渲染：空不打印、降序、只有类目与数字。
func TestFilterSweepCategoryLine(t *testing.T) {
	t.Parallel()

	if line := filterSweepCategoryLine(nil); line != "" {
		t.Fatalf("没有命中时不该打印，实际 %q", line)
	}
	if line := filterSweepCategoryLine(map[contentfilter.Category]int{}); line != "" {
		t.Fatalf("空映射时不该打印，实际 %q", line)
	}

	line := filterSweepCategoryLine(map[contentfilter.Category]int{
		contentfilter.CategoryDrugs:    3,
		contentfilter.CategoryPolitics: 168,
		contentfilter.CategoryFraud:    0,
	})
	if !strings.HasPrefix(line, "类目归因:     ") {
		t.Fatalf("报告行格式变了: %q", line)
	}
	// 标签要说"条"，不能说"处"：两者口径不同，混用会让人拿它去和合计对账。
	if strings.Contains(line, "处") {
		t.Fatalf("类目归因按实体条数计，不该写“处”：%q", line)
	}
	// 降序：条数多的在前（一眼看出问题来自哪一类）。
	if strings.Index(line, "politics 168 条") > strings.Index(line, "drugs 3 条") {
		t.Fatalf("应按条数降序，实际 %q", line)
	}
	// 0 的类目不出现，避免报告里塞一堆 0 干扰判断。
	if strings.Contains(line, "fraud") {
		t.Fatalf("0 条的类目不该出现，实际 %q", line)
	}
}

// TestSweepReportCountsEntityCategories 钉住扫描时按**实体**累计类目。
//
// 走公开入口 sweepContent（预演），断言的是"报告里能看出命中来自哪一类"，
// 且类目条数不超过该面的命中条数（同一面里每条内容对同一类目只算一次）。
func TestSweepReportCountsEntityCategories(t *testing.T) {
	db, _ := seedSweepFixture(t)
	engine := newSweepEngine(t)

	report, err := sweepContent(context.Background(), db, engine, nil, false)
	if err != nil {
		t.Fatalf("预演不该失败: %v", err)
	}
	if len(report.CategoryHits) == 0 {
		t.Fatalf("有命中时 report 应按类目累计条数，实际为空（报告：%s）", report.String())
	}

	hitRows := 0
	for _, surface := range report.Surfaces {
		hitRows += surface.Hit
	}
	categoryRows := 0
	for _, count := range report.CategoryHits {
		categoryRows += count
	}
	// 一条内容可能被多个类目打中，所以类目条数之和 >= 命中条数；
	// 但绝不该超过"命中条数 × 类目数"，否则说明同一实体被重复计数了。
	if categoryRows < hitRows {
		t.Fatalf("类目条数之和（%d）不该少于命中条数（%d）", categoryRows, hitRows)
	}
	if categoryRows > hitRows*len(contentfilter.AllCategories) {
		t.Fatalf("类目条数之和（%d）远超命中条数（%d）× 类目数，疑似同一实体被重复计数",
			categoryRows, hitRows)
	}

	// 报告里要能看到这一行（格式契约：它在 String() 里，不在外面拼）。
	if !strings.Contains(report.String(), "类目归因:") {
		t.Fatalf("报告应包含类目归因行，实际：%s", report.String())
	}
}
