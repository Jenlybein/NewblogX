package app_test

import (
	"context"
	"testing"

	"myblogx/internal/media/domain"
)

// 轮询任务状态时必须回**当前**审核结论，而不是入库那一刻的快照。
//
// 事故现场（2026-11-27）：上传一张违规图，入库与判定都在回调里，任务先落盘
// （ImageURL 此刻已非空，图片状态还是 0=unknown），审核结论随后才写进图行。
// 旧的 GetStatus 看到 ImageURL 非空就照快照拼一个 Image 返回（状态 0），客户端
// 拿到 "unknown"，提示成了「这张图状态异常，请换一张后重试」；真实结论是 block。
func TestGetStatus_回当前审核结论而不是任务快照(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, repo, _ := newAuditService(true, true)

	created, err := svc.CreateTask(ctx, 1, "a.png", 1, "image/png", "hash-1", nil)
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	confirmed, err := svc.ConfirmByUser(ctx, created.Task.ID.Uint64(), 1, created.Task.ObjectKey)
	if err != nil {
		t.Fatalf("确认上传失败: %v", err)
	}

	// 模拟审核（同步判定 / 七牛回调）在任务落盘之后给出"违规"。
	if err := repo.UpdateStatus(ctx, confirmed.Image.ID, domain.StatusBlocked); err != nil {
		t.Fatalf("改图片状态失败: %v", err)
	}

	result, err := svc.GetStatus(ctx, confirmed.Task.ID.Uint64(), 1)
	if err != nil {
		t.Fatalf("查任务状态失败: %v", err)
	}
	if result.Image == nil {
		t.Fatal("状态响应必须带图片信息，否则客户端无法区分审核结论")
	}
	if result.Image.Status != domain.StatusBlocked {
		t.Fatalf("必须回图行的当前结论 block，实际 %s", result.Image.Status.String())
	}
	if result.Image.Status.Displayable() {
		t.Fatal("block 不该可展示")
	}
}
