package cli

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/platform/contentfilter"
)

// 本文件钉住"存量清洗按类目报处数"这条误伤评估口径：
// 词表变大以后，"命中 176 处"这个总数无法判断该不该 apply ——
// 176 处来自涉政宽泛词和来自毒品垃圾词是完全不同的两件事。
//
// 约束：报告只含类目名与数字，不含词条（词表是敏感数据）。

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
	if !strings.HasPrefix(line, "类目分布:     ") {
		t.Fatalf("报告行格式变了: %q", line)
	}
	// 降序：处数多的在前（一眼看出问题来自哪一类）。
	if strings.Index(line, "politics 168 处") > strings.Index(line, "drugs 3 处") {
		t.Fatalf("应按处数降序，实际 %q", line)
	}
	// 0 处的类目不出现，避免报告里塞一堆 0 干扰判断。
	if strings.Contains(line, "fraud") {
		t.Fatalf("0 处的类目不该出现，实际 %q", line)
	}
}

// TestSweepReportCountsSpansByCategory 钉住扫描时真的按类目累计。
//
// 走公开入口 sweepContent（预演），所以断言的是"报告里能看出命中来自哪一类"。
func TestSweepReportCountsSpansByCategory(t *testing.T) {
	db, _ := seedSweepFixture(t)
	engine := newSweepEngine(t)

	report, err := sweepContent(context.Background(), db, engine, nil, false)
	if err != nil {
		t.Fatalf("预演不该失败: %v", err)
	}
	if len(report.CategorySpans) == 0 {
		t.Fatalf("有命中时 report 应按类目累计处数，实际为空（报告：%s）", report.String())
	}
	total := 0
	for _, count := range report.CategorySpans {
		total += count
	}
	spans := 0
	for _, surface := range report.Surfaces {
		spans += surface.Spans
	}
	if total != spans {
		t.Fatalf("按类目累计应等于总替换处数：类目 %d 处，总 %d 处", total, spans)
	}
}
