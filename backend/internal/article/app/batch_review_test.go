package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
)

// 批量处理：能处理的都处理掉，失败的逐条返回，不让一条失败拖垮整批。
func TestBatchHandleReviewTasksReportsPartialSuccess(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()

	// 一条真实存在的待处理任务 + 一条不存在的 ID。
	seedReviewedArticle(t, repos, 1, 11, "正文甲")

	result, err := svc.BatchHandleReviewTasks(ctx, adminViewer(), []ctype.ID{
		ctype.NewID(11),  // 存在
		ctype.NewID(999), // 不存在
	}, true, "", "")
	if err != nil {
		t.Fatalf("批量通过不该整批失败: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("成功条数 = %d，期望 1", result.Succeeded)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("失败条数 = %d，期望 1", len(result.Failed))
	}
	if result.Failed[0].TaskID != ctype.NewID(999) {
		t.Fatalf("失败项应指向不存在的任务，实际 %v", result.Failed[0].TaskID)
	}
	if result.Failed[0].Code == "" {
		t.Fatal("失败项必须带错误码，前端才知道怎么提示")
	}
}

// 批量驳回同样要求原因码：不合法时整批都不进事务，直接报错。
func TestBatchRejectRequiresReasonCode(t *testing.T) {
	t.Parallel()

	svc, repos := newModerationService(t, nil, nil)
	ctx := context.Background()
	seedReviewedArticle(t, repos, 1, 11, "正文甲")

	_, err := svc.BatchHandleReviewTasks(ctx, adminViewer(), []ctype.ID{ctype.NewID(11)},
		false, "", "")
	if err == nil {
		t.Fatal("驳回缺原因码应报错（与单条接口同一套校验）")
	}
}

// 非管理员不能批量。
func TestBatchHandleReviewTasksRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, _ := newModerationService(t, nil, nil)
	_, err := svc.BatchHandleReviewTasks(context.Background(), authorViewer(),
		[]ctype.ID{ctype.NewID(11)}, true, "", "")
	if err == nil {
		t.Fatal("非管理员不该能批量处理审核任务")
	}
	_ = app.BatchReviewResult{}
}
