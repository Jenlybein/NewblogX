package project_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件只钉"什么是合法项目"的领域规则。读口径（只出展示中 / 篇数只算公开 /
// 封面回退）属于读模型，在 adapter/mysql 与 HTTP 集成用例里断言。

func TestNewProjectTrimsTitle(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	p, err := project.New(project.NewParams{
		ID:        ctype.NewID(1),
		Title:     "  重构实战  ",
		CreatorID: domain.UserID(ctype.NewID(9)),
		Now:       at,
	})
	if err != nil {
		t.Fatalf("建项目应成功: %v", err)
	}
	if p.Title() != "重构实战" {
		t.Fatalf("标题应去掉首尾空白，实际 %q", p.Title())
	}
	if p.Displayed() {
		t.Fatalf("默认不展示：先准备、后公布")
	}
}

func TestNewProjectRejectsEmptyTitle(t *testing.T) {
	_, err := project.New(project.NewParams{ID: ctype.NewID(1), Title: "   "})
	if !errors.Is(err, project.ErrTitleRequired) {
		t.Fatalf("空标题应报 ErrTitleRequired，实际 %v", err)
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeProjectTitleRequired {
		t.Fatalf("错误码应为 %q，实际 %v/%v", apperr.CodeProjectTitleRequired, code, ok)
	}
}

func TestNewProjectRejectsTooLongTitle(t *testing.T) {
	long := make([]rune, project.MaxTitleLen+1)
	for i := range long {
		long[i] = '题'
	}
	_, err := project.New(project.NewParams{ID: ctype.NewID(1), Title: string(long)})
	if !errors.Is(err, project.ErrTitleTooLong) {
		t.Fatalf("超长标题应报错，实际 %v", err)
	}
}

func TestRestoreKeepsPersistedState(t *testing.T) {
	p := project.Restore(project.RestoreParams{
		ID:        ctype.NewID(7),
		Title:     "长系列",
		Cover:     "https://cdn.example/cover.png",
		Displayed: true,
		SortOrder: 3,
		Deleted:   true,
	})
	if p.ID() != ctype.NewID(7) || !p.Displayed() || p.SortOrder() != 3 || !p.Deleted() {
		t.Fatalf("Restore 必须原样还原库里的状态: %+v", p)
	}
}
