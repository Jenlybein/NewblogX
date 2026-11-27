package review_test

import (
	"errors"
	"myblogx/internal/kernel/ctype"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newTask() *review.Task {
	return review.NewTask(review.NewTaskParams{
		ID:            ctype.NewID(1),
		ArticleID:     ctype.NewID(10),
		AuthorID:      ctype.NewID(100),
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
		if err := task.Approve(ctype.NewID(900), "", now); err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if task.Status() != review.StatusApproved {
			t.Fatalf("状态应为 approved，实际 %s", task.Status())
		}
		if task.ReviewedBy() == nil || *task.ReviewedBy() != ctype.NewID(900) {
			t.Fatal("应记录审核人")
		}
		if task.ReviewedAt() == nil {
			t.Fatal("应记录审核时间")
		}
		// 重复处理
		if err := task.Approve(ctype.NewID(900), "", now); !errors.Is(err, review.ErrTaskHandled) {
			t.Fatalf("重复通过应报 ErrTaskHandled，实际 %v", err)
		}
	})

	t.Run("拒绝带原因", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Reject(ctype.NewID(900), review.ReasonIncomplete, "缺少示例代码", now); err != nil {
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
		if err := task.Approve(ctype.NewID(900), "", now); !errors.Is(err, review.ErrTaskHandled) {
			t.Fatalf("撤销后通过应报错，实际 %v", err)
		}
	})

	t.Run("无审核人不允许通过", func(t *testing.T) {
		t.Parallel()
		task := newTask()
		if err := task.Approve("", "", now); !errors.Is(err, review.ErrTaskForbidden) {
			t.Fatalf("期望 ErrTaskForbidden，实际 %v", err)
		}
	})
}

// TestApproveKeepsReasonEmptyAndStoresNote 通过时 reason 必须保持空、
// 放行留痕只进 approve_note：两条事实不能挤在同一列里。
func TestApproveKeepsReasonEmptyAndStoresNote(t *testing.T) {
	t.Parallel()

	task := newTask()
	note := review.FormatApproveNote(ptrInt(47), 52)
	if err := task.Approve(ctype.NewID(900), note, now); err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if task.Reason() != "" {
		t.Fatalf("通过不该在 reason 上留下驳回原因，实际 %q", task.Reason())
	}
	if task.ApproveNote() != "管理员以 47 分放行（放行线 52 分）" {
		t.Fatalf("放行留痕应为固定文案，实际 %q", task.ApproveNote())
	}
	// 重复通过沿用既有状态判断，不会覆盖/追加第二条留痕。
	if err := task.Approve(ctype.NewID(901), "再来一条", now); !errors.Is(err, review.ErrTaskHandled) {
		t.Fatalf("重复通过应报 ErrTaskHandled，实际 %v", err)
	}
	if task.ApproveNote() != "管理员以 47 分放行（放行线 52 分）" {
		t.Fatalf("重复通过不该改写留痕，实际 %q", task.ApproveNote())
	}
}

// TestFormatApproveNoteDistinguishesUnknownFromZero 钉住"取不到分数"与"0 分"
// 不是一句话：nil 是未知（AI 关闭 / 未评分 / 模型失败），0 是模型真的给了 0 分。
func TestFormatApproveNoteDistinguishesUnknownFromZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		score *int
		want  string
	}{
		{"有分数", ptrInt(47), "管理员以 47 分放行（放行线 52 分）"},
		{"零分不等于未知", ptrInt(0), "管理员以 0 分放行（放行线 52 分）"},
		{"取不到分数", nil, "管理员以未知分数放行（放行线 52 分）"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := review.FormatApproveNote(c.score, 52); got != c.want {
				t.Fatalf("格式应为 %q，实际 %q", c.want, got)
			}
		})
	}
}

// ptrInt 取分数的地址：*int 是"可能没有分数"的表达（nil ≠ 0）。
func ptrInt(v int) *int { return &v }
