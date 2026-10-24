package apptest

import (
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// RequireVisible 断言某角色能看到文章。
func RequireVisible(t *testing.T, a *domain.Article, v domain.Viewer) {
	t.Helper()
	if !domain.CanBeViewedBy(a, v) {
		t.Fatalf("期望可见（文章状态=%s 可见性=%s 已删=%v，角色=%+v）",
			a.Status(), a.Visibility(), a.Deleted(), v)
	}
}

// RequireNotVisible 断言某角色看不到文章。
func RequireNotVisible(t *testing.T, a *domain.Article, v domain.Viewer) {
	t.Helper()
	if domain.CanBeViewedBy(a, v) {
		t.Fatalf("期望不可见（文章状态=%s 可见性=%s 已删=%v，角色=%+v）",
			a.Status(), a.Visibility(), a.Deleted(), v)
	}
}

// RequireErrorCode 断言错误链里包含指定错误码。
//
// 比 errors.Is(err, domain.ErrXxx) 更适合跨层断言：adapter 可能包了一层，
// 但错误码必须原样透出。
func RequireErrorCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际没有错误", code)
	}
	got, ok := apperr.CodeOf(err)
	if !ok {
		t.Fatalf("期望错误码 %s，实际错误无码: %v", code, err)
	}
	if got != code {
		t.Fatalf("期望错误码 %s，实际 %s（%v）", code, got, err)
	}
}

// RequireNoError 断言没有错误。
func RequireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
}

// RequireError 断言有错误。
func RequireError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("期望报错，实际通过")
	}
}

// RequireErrorIs 断言错误可用 errors.Is 命中。
func RequireErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("期望 %v，实际 %v", target, err)
	}
}
