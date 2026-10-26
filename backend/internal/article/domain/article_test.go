package domain_test

import (
	"errors"
	"myblogx/internal/kernel/ctype"
	"testing"
	"time"

	"myblogx/internal/article/domain"
)

var baseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newDraft(t *testing.T) *domain.Article {
	t.Helper()
	a, err := domain.NewArticle(domain.NewArticleParams{
		ID:       ctype.NewID(10),
		AuthorID: authorID,
		Title:    "标题",
		Content:  "正文",
		Status:   domain.StatusDraft,
		Now:      baseTime,
	})
	if err != nil {
		t.Fatalf("构造文章失败: %v", err)
	}
	return a
}

func TestNewArticleValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		params  domain.NewArticleParams
		wantErr error
	}{
		{
			name:    "缺作者",
			params:  domain.NewArticleParams{AuthorID: "", Title: "t", Status: domain.StatusDraft},
			wantErr: domain.ErrForbidden,
		},
		{
			name:    "空标题",
			params:  domain.NewArticleParams{AuthorID: authorID, Title: "   ", Status: domain.StatusDraft},
			wantErr: domain.ErrTitleRequired,
		},
		{
			name:    "非法状态",
			params:  domain.NewArticleParams{AuthorID: authorID, Title: "t", Status: domain.Status(99)},
			wantErr: domain.ErrInvalidTransition,
		},
		{
			name:   "正常",
			params: domain.NewArticleParams{AuthorID: authorID, Title: "t", Status: domain.StatusDraft},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewArticle(c.params)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("不应报错，实际: %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("期望 %v，实际 %v", c.wantErr, err)
			}
		})
	}
}

// TestSubmitTransitions 覆盖"提交"的所有来源状态与站点开关组合。
func TestSubmitTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		from          domain.Status
		skipExamining bool
		want          domain.Status
		wantErr       error
	}{
		{"草稿/需审核", domain.StatusDraft, false, domain.StatusExamining, nil},
		{"草稿/跳过审核", domain.StatusDraft, true, domain.StatusPublished, nil},
		{"已拒绝/需审核", domain.StatusRejected, false, domain.StatusExamining, nil},
		{"已拒绝/跳过审核", domain.StatusRejected, true, domain.StatusPublished, nil},
		{"审核中/重提", domain.StatusExamining, false, domain.StatusExamining, nil},
		{"已发布/需重审", domain.StatusPublished, false, domain.StatusExamining, nil},
		{"已发布/跳过审核应报错", domain.StatusPublished, true, domain.StatusPublished, domain.ErrAlreadyPublished},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newArticle(t, c.from, domain.VisibilityVisible, false)
			err := a.Submit(baseTime.Add(time.Hour), c.skipExamining)

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("期望错误 %v，实际 %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if a.Status() != c.want {
				t.Fatalf("状态应为 %s，实际 %s", c.want, a.Status())
			}
			if a.SubmittedAt() == nil {
				t.Fatal("提交后应记录 submitted_at")
			}
			if a.ReviewedAt() != nil || a.ReviewedBy() != nil {
				t.Fatal("重新提交应清空上一轮审核痕迹")
			}
		})
	}
}

func TestApproveAndReject(t *testing.T) {
	t.Parallel()

	t.Run("审核通过", func(t *testing.T) {
		t.Parallel()
		a := newArticle(t, domain.StatusExamining, domain.VisibilityVisible, false)
		if err := a.Approve(adminID, baseTime.Add(time.Hour)); err != nil {
			t.Fatalf("审核通过失败: %v", err)
		}
		if a.Status() != domain.StatusPublished {
			t.Fatalf("状态应为已发布，实际 %s", a.Status())
		}
		if a.ReviewedBy() == nil || *a.ReviewedBy() != adminID {
			t.Fatal("应记录审核人")
		}
		if a.ReviewedAt() == nil {
			t.Fatal("应记录审核时间")
		}
	})

	t.Run("审核拒绝", func(t *testing.T) {
		t.Parallel()
		a := newArticle(t, domain.StatusExamining, domain.VisibilityVisible, false)
		if err := a.Reject(adminID, baseTime.Add(time.Hour)); err != nil {
			t.Fatalf("审核拒绝失败: %v", err)
		}
		if a.Status() != domain.StatusRejected {
			t.Fatalf("状态应为已拒绝，实际 %s", a.Status())
		}
	})

	t.Run("草稿不能审核", func(t *testing.T) {
		t.Parallel()
		a := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)
		if err := a.Approve(adminID, baseTime); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("期望 ErrInvalidTransition，实际 %v", err)
		}
		if err := a.Reject(adminID, baseTime); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("期望 ErrInvalidTransition，实际 %v", err)
		}
	})
}

func TestBackToDraftClearsReviewTrail(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusExamining, domain.VisibilityVisible, false)
	if err := a.BackToDraft(baseTime.Add(time.Hour)); err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if a.Status() != domain.StatusDraft {
		t.Fatalf("状态应为草稿，实际 %s", a.Status())
	}
	if a.SubmittedAt() != nil || a.ReviewedAt() != nil || a.ReviewedBy() != nil {
		t.Fatal("退回草稿应清空提交与审核痕迹")
	}

	// 已经是草稿时再退一次不应报错（幂等）。
	if err := a.BackToDraft(baseTime); err != nil {
		t.Fatalf("重复退回草稿应幂等: %v", err)
	}
}

// TestNeedsReReview 锁住"编辑后是否重新送审"的判定，含"只改可见性不算"这条边界。
func TestNeedsReReview(t *testing.T) {
	t.Parallel()

	onlyVisibility := domain.EditFlags{Visibility: true}
	content := domain.EditFlags{Content: true}
	tags := domain.EditFlags{Tags: true}

	cases := []struct {
		name          string
		status        domain.Status
		flags         domain.EditFlags
		skipExamining bool
		want          bool
	}{
		{"已发布+改正文", domain.StatusPublished, content, false, true},
		{"已发布+只改可见性", domain.StatusPublished, onlyVisibility, false, false},
		{"审核中+改标签", domain.StatusExamining, tags, false, true},
		{"草稿+改正文", domain.StatusDraft, content, false, false},
		{"已拒绝+改正文", domain.StatusRejected, content, false, false},
		{"跳过审核时永不重审", domain.StatusPublished, content, true, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newArticle(t, c.status, domain.VisibilityVisible, false)
			if got := a.NeedsReReview(c.flags, c.skipExamining); got != c.want {
				t.Fatalf("NeedsReReview = %v，期望 %v", got, c.want)
			}
		})
	}
}

func TestApplyEdit(t *testing.T) {
	t.Parallel()

	t.Run("无变化不刷新 updatedAt", func(t *testing.T) {
		t.Parallel()
		a := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)
		same := "测试文章"
		flags, err := a.ApplyEdit(domain.EditPatch{Title: &same}, baseTime.Add(time.Hour))
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if flags != (domain.EditFlags{}) {
			t.Fatalf("值没变不应标记变更: %+v", flags)
		}
		if !a.UpdatedAt().Equal(baseTime) {
			t.Fatal("无实质变化不应刷新 updated_at")
		}
	})

	t.Run("清空分类", func(t *testing.T) {
		t.Parallel()
		cid := ctype.NewID(7)
		a := domain.Restore(domain.RestoreParams{
			ID: ctype.NewID(1), AuthorID: authorID, Title: "t",
			CategoryID: &cid, Status: domain.StatusDraft, CreatedAt: baseTime, UpdatedAt: baseTime,
		})
		flags, err := a.ApplyEdit(domain.EditPatch{CategorySet: true, CategoryID: nil}, baseTime)
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if !flags.Category {
			t.Fatal("应标记分类变更")
		}
		if a.CategoryID() != nil {
			t.Fatal("分类应被清空")
		}
	})

	t.Run("空标题拒绝", func(t *testing.T) {
		t.Parallel()
		a := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)
		empty := "  "
		if _, err := a.ApplyEdit(domain.EditPatch{Title: &empty}, baseTime); !errors.Is(err, domain.ErrTitleRequired) {
			t.Fatalf("期望 ErrTitleRequired，实际 %v", err)
		}
	})
}

// TestRestoreDoesNotValidate 保证历史脏数据仍能读出（否则一条坏数据能让整页 500）。
func TestRestoreDoesNotValidate(t *testing.T) {
	t.Parallel()

	a := domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(1),
		AuthorID:   "",        // 非法：无作者
		Title:      "",        // 非法：空标题
		Visibility: "garbage", // 非法：未知可见性
		Status:     domain.StatusPublished,
	})

	if a.Visibility() != domain.VisibilityVisible {
		t.Fatalf("非法可见性应归一为 visible，实际 %s", a.Visibility())
	}
	if a.Title() != "" {
		t.Fatal("Restore 不应改写数据")
	}
}

func TestSoftDeleteHidesArticle(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)
	if !domain.CanBeViewedBy(a, domain.Anonymous()) {
		t.Fatal("删除前匿名可见")
	}

	a.SoftDelete(baseTime.Add(time.Hour))

	if !a.Deleted() {
		t.Fatal("应标记为已删除")
	}
	if domain.CanBeViewedBy(a, domain.Anonymous()) || domain.CanBeViewedBy(a, domain.AdminUser(adminID)) {
		t.Fatal("软删后任何人都不可见（与旧实现查询不到等价）")
	}
	if err := a.Submit(baseTime, false); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("已删除文章不能提交，期望 ErrArticleNotFound，实际 %v", err)
	}
}

func TestPageNormalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   domain.Page
		want domain.Page
	}{
		{domain.Page{Offset: -1, Limit: 0}, domain.Page{Offset: 0, Limit: 10}},
		{domain.Page{Offset: 5, Limit: 500}, domain.Page{Offset: 5, Limit: 100}},
		{domain.Page{Offset: 5, Limit: 20}, domain.Page{Offset: 5, Limit: 20}},
	}

	for _, c := range cases {
		if got := c.in.Normalize(); got != c.want {
			t.Fatalf("%+v 归一后应为 %+v，实际 %+v", c.in, c.want, got)
		}
	}
}
