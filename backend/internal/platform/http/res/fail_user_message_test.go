package res_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/user/domain"
)

// 共享响应层的「动态文案」通道（code-review 修复）。
//
// 规则只有一条，但很容易被后来的改动放宽成"任何错误都能自带文案"（那会把
// 连接串、SQL、内部路径带进响应）：允许实现 `UserMessage()` 的错误覆盖文案，
// **仅限 ProtocolValidation**。下面用两个真实的错误值把两侧都钉住。

// TestFailUsesMuteUserMessage 禁言错误的到期时间与剩余天数必须真的到用户面前。
func TestFailUsesMuteUserMessage(t *testing.T) {
	until := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	muteErr := domain.NewMuteError(until, time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))

	c, w := newGinCtx()
	res.Fail(muteErr, c)
	body := decodeBody(t, w)

	if body["code"].(float64) != float64(res.FailValidCode) {
		t.Fatalf("禁言是调用方能修正的失败，应归 1001，实际 %v", body["code"])
	}
	msg := body["msg"].(string)
	// 静态文案只有「账号已被禁言」；动态文案带到期时间与剩余天数。
	if msg == apperr.Message(apperr.CodeUserMuted) {
		t.Fatalf("不该退化成消息表里的静态文案: %q", msg)
	}
	if want := "2026-11-24 12:00"; !contains(msg, want) {
		t.Fatalf("文案应带上到期时间 %s，实际 %q", want, msg)
	}
}

// TestFailIgnoresUserMessageOnServiceErrors 服务异常的自带文案必须被忽略。
//
// 这条是上一道口子的护栏：如果哪天有人给一个 ProtocolService 的错误加了
// UserMessage()，这里会红 —— 因为那种错误的 Error() 里可能是连接串。
func TestFailIgnoresUserMessageOnServiceErrors(t *testing.T) {
	c, w := newGinCtx()
	res.Fail(serviceErrorWithMessage{}, c)
	body := decodeBody(t, w)

	if body["code"].(float64) != float64(res.FailServiceCode) {
		t.Fatalf("未知/服务异常应归 1002，实际 %v", body["code"])
	}
	if got := body["msg"].(string); got != apperr.Message(apperr.CodeCommonServiceFailed) {
		t.Fatalf("服务异常不该用错误自带的文案，实际 %q", got)
	}
}

// serviceErrorWithMessage 是一个"自带文案的服务异常"（模拟内部错误）。
type serviceErrorWithMessage struct{}

func (serviceErrorWithMessage) Error() string        { return "dial tcp 10.0.0.5:3306: connect: refused" }
func (serviceErrorWithMessage) UserMessage() string  { return "dial tcp 10.0.0.5:3306: connect: refused" }
func (serviceErrorWithMessage) Is(target error) bool { return target == errors.New("") }

// contains 是 strings.Contains 的小包装（避免为一条断言再引一个包）。
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
