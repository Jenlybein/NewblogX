package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/media/app"
	"myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
)

// 本文件是 ADR-0034「审核结论的两条来源」在用例层的接缝：
//   - 回调走不通时，主动审核能把图推成通过；
//   - 主动审核**失败**时图保持待审核，不能当成通过；
//   - 超时扫描把积压推进人工队列，并把疑似图在存储侧禁用；
//   - 人工放行只对疑似/待审生效，且会解禁；违规图不可放行。

// censorStorage 让用例指定主动审核的结论（或让它失败）。
type censorStorage struct {
	apptest.Storage
	suggestion string
	err        error
	enabled    []bool
	deleted    []string
}

func (s *censorStorage) Delete(_, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *censorStorage) Censor(_ context.Context, _, _ string) (string, error) {
	return s.suggestion, s.err
}

func (s *censorStorage) SetEnabled(_ context.Context, _, _ string, enabled bool) error {
	s.enabled = append(s.enabled, enabled)
	return nil
}

func (s *censorStorage) Stat(_, _ string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Hash: "hash-1", Size: 1}, nil
}

func (s *censorStorage) ImageInfo(_ string) (domain.ImageInfo, error) {
	return domain.ImageInfo{Format: "png", Width: 1600, Height: 900, Size: 1}, nil
}

// newCensoredService 组装一个审核开启、主动审核结论可控的服务。
func newCensoredService(storage *censorStorage) (*app.Service, *apptest.Repo) {
	cfg := purposeCfg()
	cfg.AuditEnabled = true
	cfg.AuditAutoDelete = true

	repo := apptest.NewRepo()
	svc := app.New(repo, apptest.NewTaskStore(), apptest.NewAuditStore(), storage,
		&apptest.IDs{}, apptest.Clock{}, nil, func() domain.Config { return cfg })
	return svc, repo
}

// TestCensorNow_主动审核放行 钉住"不依赖回调也能出结论"。
//
// 这条是域名封禁期的生命线：回调进不来时，只有主动审核能把图从待审核推走。
func TestCensorNow_主动审核放行(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{suggestion: "pass"}
	svc, repo := newCensoredService(storage)

	image := createAndConfirm(t, svc)
	if image.Status != domain.StatusPass {
		t.Fatalf("主动审核给出 pass 后应为 pass，实际 %s", image.Status.String())
	}
	got, _ := repo.FindByID(context.Background(), image.ID)
	if got.Status != domain.StatusPass {
		t.Fatalf("库里应为 pass，实际 %s", got.Status.String())
	}
}

// TestCensorNow_审核不可用时保持待审核 钉住 fail closed。
func TestCensorNow_审核不可用时保持待审核(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{err: context.DeadlineExceeded}
	svc, _ := newCensoredService(storage)

	image := createAndConfirm(t, svc)
	if image.Status != domain.StatusPending {
		t.Fatalf("审核调用失败时应保持待审核，实际 %s", image.Status.String())
	}
}

// TestSweepAuditTimeout_积压转人工 钉住"等太久也不放行，只转人工"。
func TestSweepAuditTimeout_积压转人工(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{err: context.DeadlineExceeded}
	svc, repo := newCensoredService(storage)
	image := createAndConfirm(t, svc)

	// 用"未来时刻"当截止线，让这张刚建的图也算超时。
	moved, err := svc.SweepAuditTimeout(context.Background(), futureUnix())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if moved == 0 {
		t.Fatal("应至少把这张待审核的图转进人工队列")
	}

	got, _ := repo.FindByID(context.Background(), image.ID)
	if got.Status != domain.StatusReviewing {
		t.Fatalf("应转为疑似（人工队列），实际 %s", got.Status.String())
	}
	// 疑似图必须在存储侧也关掉：桶是公开的，只靠"不给地址"不够。
	if len(storage.enabled) == 0 || storage.enabled[0] {
		t.Fatalf("疑似图应被禁用，实际调用 %v", storage.enabled)
	}
}

// TestApproveAudit_放行并解禁 钉住人工放行的完整副作用。
func TestApproveAudit_放行并解禁(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{err: context.DeadlineExceeded}
	svc, _ := newCensoredService(storage)
	image := createAndConfirm(t, svc)

	if _, err := svc.ApproveAudit(context.Background(), image.ID); err != nil {
		t.Fatalf("放行失败: %v", err)
	}
	if len(storage.enabled) != 1 || !storage.enabled[0] {
		t.Fatalf("放行时应解禁对象，实际调用 %v", storage.enabled)
	}
}

// TestApproveAudit_违规图不可放行 钉住"blocked 是终局"在人工入口同样成立。
func TestApproveAudit_违规图不可放行(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{suggestion: "block"}
	svc, _ := newCensoredService(storage)
	image := createAndConfirm(t, svc)

	if image.Status != domain.StatusBlocked {
		t.Fatalf("应为违规，实际 %s", image.Status.String())
	}
	if _, err := svc.ApproveAudit(context.Background(), image.ID); err == nil {
		t.Fatal("违规图不应允许放行")
	}
}

// TestRejectAudit_驳回即删除 钉住人工驳回走同一条副作用链。
func TestRejectAudit_驳回即删除(t *testing.T) {
	t.Parallel()

	storage := &censorStorage{err: context.DeadlineExceeded}
	svc, repo := newCensoredService(storage)
	image := createAndConfirm(t, svc)

	if _, err := svc.RejectAudit(context.Background(), image.ID); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	got, _ := repo.FindByID(context.Background(), image.ID)
	if got.Status != domain.StatusBlocked {
		t.Fatalf("驳回后应为违规，实际 %s", got.Status.String())
	}
	if len(storage.deleted) != 1 {
		t.Fatalf("驳回应删除对象，实际 %v", storage.deleted)
	}
}

// futureUnix 返回一个足够大的截止时刻（让"刚建好的图"也算超时）。
func futureUnix() int64 {
	return apptest.Clock{}.Now().Add(24 * time.Hour).Unix()
}
