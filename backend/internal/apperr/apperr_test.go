package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"myblogx/internal/apperr"
)

func TestErrorCarriesCodeAndMessage(t *testing.T) {
	t.Parallel()

	err := apperr.New(apperr.CodeArticleNotFound)
	if err.Code() != apperr.CodeArticleNotFound {
		t.Fatalf("错误码应为 %s，实际 %s", apperr.CodeArticleNotFound, err.Code())
	}
	if err.Error() != "文章不存在" {
		t.Fatalf("文案应来自消息表，实际 %q", err.Error())
	}
}

// TestErrorsIsByCode 保证跨包比较用错误码而不是实例指针。
func TestErrorsIsByCode(t *testing.T) {
	t.Parallel()

	err := apperr.Wrap(apperr.CodeArticleNotFound, errors.New("row not found"))
	if !errors.Is(err, apperr.New(apperr.CodeArticleNotFound)) {
		t.Fatal("同码不同实例应被判为同一错误")
	}
	if errors.Is(err, apperr.New(apperr.CodeArticleForbidden)) {
		t.Fatal("不同码不应相等")
	}

	cause := errors.New("row not found")
	if !errors.Is(apperr.Wrap(apperr.CodeArticleNotFound, cause), cause) {
		t.Fatal("应能穿透到原始原因")
	}
}

func TestCodeOfThroughWrapping(t *testing.T) {
	t.Parallel()

	base := apperr.New(apperr.CodeTagDisabled)
	wrapped := fmt.Errorf("保存标签失败: %w", base)

	code, ok := apperr.CodeOf(wrapped)
	if !ok || code != apperr.CodeTagDisabled {
		t.Fatalf("应能穿透 fmt.Errorf 拿到错误码，实际 %v %v", code, ok)
	}
	if !apperr.IsCode(wrapped, apperr.CodeTagDisabled) {
		t.Fatal("IsCode 应返回 true")
	}
}

// TestMissingMessageFallsBackToCode 防"新增错误码忘了补文案"时静默变成空串。
func TestMissingMessageFallsBackToCode(t *testing.T) {
	t.Parallel()

	err := apperr.New(apperr.Code("article.not_configured"))
	if err.Error() != "article.not_configured" {
		t.Fatalf("未配置文案时应退化为码本身，实际 %q", err.Error())
	}
}

func TestMessage(t *testing.T) {
	t.Parallel()

	if got := apperr.Message(apperr.CodeArticleTitleRequired); got != "标题不能为空" {
		t.Fatalf("文案不符: %q", got)
	}
}
