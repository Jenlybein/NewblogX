package review_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newTask() *review.Task {
	return review.NewTask(review.NewTaskParams{
		ID:            1,
		ArticleID:     domain.ArticleID(10),
		AuthorID:      domain.UserID(100),
		ArticleTitle:  "待审文章",
		AuthorName:    "作者",
		PublishStatus: domain.StatusExamining,
		Source:        review.SourceCreate,
		Now:           now,
	})
}

func TestNewTaskIsPending(t *testing.T) {
	t.Parallel()

	task := newTask()
	if !task.IsPending() {
		t.Fatal("新建任务应为待处理")
	}
	if task.Status() != review.StatusPending {
		t.Fatalf("状态应为 pending，实际 %s", task.Status())
	}
}

// TestTaskStateMachine 覆盖 待处理 → 通过/拒绝/撤销，以及重复处理必须报错。
func TestTaskStateMachine(t *testing.T) {
	t.Parallel()

	t.Run("通过", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Approve(domain.UserID(900), now); err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if task.Status() != review.StatusApproved {
			t.Fatalf("状态应为 approved，实际 %s", task.Status())
		}
		if task.ReviewedBy() == nil || *task.ReviewedBy() != domain.UserID(900) {
			t.Fatal("应记录审核人")
		}
		if task.ReviewedAt() == nil {
			t.Fatal("应记录审核时间")
		}
		// 重复处理
		if err := task.Approve(domain.UserID(900), now); !errors.Is(err, review.ErrTaskHandled) {
			t.Fatalf("重复通过应报 ErrTaskHandled，实际 %v", err)
		}
	})

	t.Run("拒绝带原因", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Reject(domain.UserID(900), "内容不完整", now); err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if task.Status() != review.StatusRejected {
			t.Fatalf("状态应为 rejected，实际 %s", task.Status())
		}
	})

	t.Run("撤销", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Cancel("作者改回草稿", now); err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if task.Status() != review.StatusCanceled {
			t.Fatalf("状态应为 canceled，实际 %s", task.Status())
		}
		// 撤销后不能再通过
		if err := task.Approve(domain.UserID(900), now); !errors.Is(err, review.ErrTaskHandled) {
			t.Fatalf("撤销后通过应报错，实际 %v", err)
		}
	})

	t.Run("无审核人不允许通过", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Approve(0, now); !errors.Is(err, review.ErrTaskForbidden) {
			t.Fatalf("期望 ErrTaskForbidden，实际 %v", err)
		}
	})
}
