package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 合规结论覆盖写：一篇文章只有一行，后写的覆盖先写的（票 312 / ADR-0033）。
func TestSaveComplianceResultOverwritesSingleRow(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()
	articleID := ctype.NewID(7001)

	if err := svc.SaveComplianceResult(ctx, articleID, app.ComplianceResult{
		Verdict:    "violation",
		RiskLevel:  3,
		Categories: []string{"广告引流"},
		Reasons:    []string{"正文含引导加微信"},
	}); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	if err := svc.SaveComplianceResult(ctx, articleID, app.ComplianceResult{
		Verdict:    "clean",
		RiskLevel:  0,
		Categories: nil,
		Reasons:    nil,
	}); err != nil {
		t.Fatalf("覆盖写失败: %v", err)
	}

	records := repos.FakeRisks().All()
	if len(records) != 1 {
		t.Fatalf("一篇文章应只有一行，实际 %d 行", len(records))
	}
	if records[0].Verdict != "clean" {
		t.Fatalf("后写的结论应覆盖先写的，实际 %s", records[0].Verdict)
	}
	if records[0].CategoriesJSON != "[]" {
		t.Fatalf("空类别应写成空数组而不是 null，实际 %s", records[0].CategoriesJSON)
	}
}

// 未知档位收敛到 suspect，而不是 clean 或 violation。
func TestSaveComplianceResultNormalizesUnknownVerdict(t *testing.T) {
	t.Parallel()

	svc, _ := newModerationService(t, nil, nil)
	ctx := context.Background()
	articleID := ctype.NewID(7002)

	if err := svc.SaveComplianceResult(ctx, articleID, app.ComplianceResult{
		Verdict:   "ok", // 模型偶尔给同义词，认不出来的一律交人看
		RiskLevel: 9,   // 越界级别收敛到 3
	}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	record, err := svc.ComplianceOf(ctx, articleID)
	if err != nil || record == nil {
		t.Fatalf("应读到记录: %v", err)
	}
	if record.Verdict != "suspect" {
		t.Fatalf("未知档位应收敛到 suspect，实际 %s", record.Verdict)
	}
	if record.RiskLevel != 3 {
		t.Fatalf("越界级别应收敛到 3，实际 %d", record.RiskLevel)
	}
}

// 审核列表带出合规结论；**没有记录必须是 nil（未判定）**，不能伪装成干净。
func TestListReviewTasksDistinguishesUncheckedFromClean(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()

	// 两篇文章：一篇判定过且干净，一篇从未判定。
	clean := seedReviewedArticle(t, repos, 1, 11, "正文甲")
	unchecked := seedReviewedArticle(t, repos, 2, 22, "正文乙")

	if err := svc.SaveComplianceResult(ctx, clean.ID(), app.ComplianceResult{
		Verdict:   "clean",
		RiskLevel: 0,
	}); err != nil {
		t.Fatalf("写入干净结论失败: %v", err)
	}

	views, _, err := svc.ListReviewTasks(ctx, adminViewer(), nil, app.ReviewSortCreatedAt, domain.Page{Limit: 50})
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}

	var foundClean, foundUnchecked bool
	for _, view := range views {
		switch view.Task.ArticleID() {
		case clean.ID():
			foundClean = true
			if view.Compliance == nil || view.Compliance.Verdict != "clean" {
				t.Fatalf("判定过的文章应带出 clean 结论，实际 %+v", view.Compliance)
			}
		case unchecked.ID():
			foundUnchecked = true
			if view.Compliance != nil {
				t.Fatalf("未判定的文章必须是 nil，实际 %+v", view.Compliance)
			}
		}
	}
	if !foundClean || !foundUnchecked {
		t.Fatalf("两篇文章都应出现在列表里（clean=%v unchecked=%v）", foundClean, foundUnchecked)
	}
}
