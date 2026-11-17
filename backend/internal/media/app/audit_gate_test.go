package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"myblogx/internal/media/app"
	"myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
)

// 本文件是 ADR-0034「审核闸门」在用例层的接缝：
//   - 审核开启时，新图入库必须是 pending（不可见），不是"先通过再改"；
//   - 审核关闭时保持老行为（入库即可见）；
//   - blocked 是终局：任何来源都不能把它改回 pass；
//   - 判定违规且开关打开时，对象存储里的文件被删掉。
//
// 断言的是用例的可观察结果（图片行的 status、对象存储收到的删除请求），
// 不是"applyStatus 里先调了哪个方法"。

// auditStorage 记录删除调用，用来验证"违规即删对象"。
type auditStorage struct {
	apptest.Storage
	deleted []string
}

func (s *auditStorage) Delete(_, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *auditStorage) Stat(_, _ string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Hash: "hash-1", Size: 1}, nil
}

func (s *auditStorage) ImageInfo(_ string) (domain.ImageInfo, error) {
	return domain.ImageInfo{Format: "png", Width: 1600, Height: 900, Size: 1}, nil
}

// newAuditService 组装一个"审核开启/关闭可切换"的服务。
func newAuditService(enabled, autoDelete bool) (*app.Service, *apptest.Repo, *auditStorage) {
	cfg := purposeCfg()
	cfg.AuditEnabled = enabled
	cfg.AuditAutoDelete = autoDelete

	repo := apptest.NewRepo()
	storage := &auditStorage{}
	svc := app.New(repo, apptest.NewTaskStore(), apptest.NewAuditStore(), storage,
		&apptest.IDs{}, apptest.Clock{}, nil, func() domain.Config { return cfg })
	return svc, repo, storage
}

// createAndConfirm 走一遍"建任务 → 确认"的真实链路，返回落地的那张图。
func createAndConfirm(t *testing.T, svc *app.Service) *domain.Image {
	t.Helper()
	ctx := context.Background()

	// hash/size 必须与 auditStorage.Stat 的返回值一致（verify 会逐项比对真实对象）。
	created, err := svc.CreateTask(ctx, 1, "a.png", 1, "image/png", "hash-1", nil)
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	result, err := svc.ConfirmByUser(ctx, created.Task.ID.Uint64(), 1, created.Task.ObjectKey)
	if err != nil {
		t.Fatalf("确认上传失败: %v", err)
	}
	return result.Image
}

// TestInitialStatus_审核开启时入库为待审核 钉住"默认不可见"。
//
// 这是整个合规闸门的起点：如果新图默认还是 pass，那么"审核回来之前不能展示"
// 这条要求从一开始就不成立，后面所有闸门都是摆设。
func TestInitialStatus_审核开启时入库为待审核(t *testing.T) {
	t.Parallel()

	svc, _, _ := newAuditService(true, true)
	image := createAndConfirm(t, svc)

	if image.Status != domain.StatusPending {
		t.Fatalf("审核开启时新图应为 pending，实际 %s", image.Status.String())
	}
	if image.Status.Displayable() {
		t.Fatal("pending 不应可展示")
	}
}

// TestInitialStatus_审核关闭时保持老行为 钉住"关掉审核等于恢复上传即可见"。
func TestInitialStatus_审核关闭时保持老行为(t *testing.T) {
	t.Parallel()

	svc, _, _ := newAuditService(false, true)
	image := createAndConfirm(t, svc)

	if image.Status != domain.StatusPass {
		t.Fatalf("审核关闭时新图应为 pass，实际 %s", image.Status.String())
	}
}

// auditBody 造一份最小的审核回调 JSON（兼容多层嵌套的 suggestion）。
func auditBody(objectKey, suggestion string) []byte {
	payload := map[string]any{
		"inputKey": objectKey,
		"result": map[string]any{
			"result": map[string]any{"suggestion": suggestion},
		},
	}
	raw, _ := json.Marshal(payload)
	return raw
}

// TestHandleAuditCallback_通过后可见 钉住"回调放行"这条主路径。
func TestHandleAuditCallback_通过后可见(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newAuditService(true, true)
	image := createAndConfirm(t, svc)

	if err := svc.HandleAuditCallback(context.Background(), auditBody(image.ObjectKey, "pass")); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}

	got, ok, err := repo.FindByObjectKey(context.Background(), image.ObjectKey)
	if err != nil || !ok {
		t.Fatalf("重查图片失败: ok=%v err=%v", ok, err)
	}
	if got.Status != domain.StatusPass {
		t.Fatalf("回调 pass 后应为 pass，实际 %s", got.Status.String())
	}
}

// TestHandleAuditCallback_违规即删对象 钉住"判定违规立刻删掉存储里的文件"。
//
// 为什么必须删：桶是公开空间，只要文件还在，任何拿到链接的人都能打开 ——
// 应用层"不给地址"只是弱保证。
func TestHandleAuditCallback_违规即删对象(t *testing.T) {
	t.Parallel()

	svc, repo, storage := newAuditService(true, true)
	image := createAndConfirm(t, svc)

	if err := svc.HandleAuditCallback(context.Background(), auditBody(image.ObjectKey, "block")); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}

	got, _, _ := repo.FindByObjectKey(context.Background(), image.ObjectKey)
	if got.Status != domain.StatusBlocked {
		t.Fatalf("应为 blocked，实际 %s", got.Status.String())
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != image.ObjectKey {
		t.Fatalf("应删除该对象，实际 %v", storage.deleted)
	}
}

// TestHandleAuditCallback_违规后不可被放行 钉住"blocked 是终局"。
//
// 回调会重试、也可能乱序（先 block 后 pass）。如果没有这条规则，
// 一次乱序就能把已经删了对象的违规图重新标成"通过"。
func TestHandleAuditCallback_违规后不可被放行(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newAuditService(true, true)
	image := createAndConfirm(t, svc)

	ctx := context.Background()
	if err := svc.HandleAuditCallback(ctx, auditBody(image.ObjectKey, "block")); err != nil {
		t.Fatalf("处理违规回调失败: %v", err)
	}
	// 模拟乱序：随后又来了一条 pass。
	if err := svc.HandleAuditCallback(ctx, auditBody(image.ObjectKey, "pass")); err != nil {
		t.Fatalf("处理乱序回调失败: %v", err)
	}

	got, _, _ := repo.FindByObjectKey(ctx, image.ObjectKey)
	if got.Status != domain.StatusBlocked {
		t.Fatalf("blocked 之后不应被改回 pass，实际 %s", got.Status.String())
	}
}

// TestHandleAuditCallback_审核关闭时不删对象 钉住开关确实生效。
func TestHandleAuditCallback_审核关闭时不删对象(t *testing.T) {
	t.Parallel()

	svc, _, storage := newAuditService(true, false)
	image := createAndConfirm(t, svc)

	if err := svc.HandleAuditCallback(context.Background(), auditBody(image.ObjectKey, "block")); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("关掉自动删除时不应删对象，实际 %v", storage.deleted)
	}
}
