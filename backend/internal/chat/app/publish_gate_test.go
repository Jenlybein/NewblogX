package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	userdomain "myblogx/internal/user/domain"
)

// 私信的发送闸门（票 399 / ADR-0032 决策 8）：
//   - 被禁言者不能发送私信，错误值原样向上返回（user.muted + 到期时间）；
//   - 读面（会话/消息列表、已读、删除）不受影响 —— 禁言只拦"输出"。

// fakePublishGate 记录被问询的用户并返回固定错误。
type fakePublishGate struct {
	err    error
	called []ctype.ID
}

func (g *fakePublishGate) EnsureCanPublish(_ context.Context, userID ctype.ID) error {
	g.called = append(g.called, userID)
	return g.err
}

func mutedChatGate() *fakePublishGate {
	return &fakePublishGate{err: userdomain.NewMuteError(
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
	)}
}

func TestSendRejectsMutedSender(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t, 2, domain.StrangerAllowed)
	svc.WithPublishGate(mutedChatGate())

	_, err := svc.Send(context.Background(), ctype.NewID(1), ctype.NewID(2),
		domain.TypeText, "禁言期间的私信", "")
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("应返回 user.muted，实际 %v", err)
	}
	// 文案带到期时间：前端把后端文案直接展示给用户。
	if !strings.Contains(err.Error(), "2026-11-01") {
		t.Fatalf("文案应含到期时间，实际 %q", err.Error())
	}
	if len(repo.sent) != 0 {
		t.Fatalf("被拒的私信不该落库，实际 %d 条", len(repo.sent))
	}
}

func TestPublishGateDoesNotAffectChatReads(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, 2, domain.StrangerAllowed)
	gate := mutedChatGate()
	svc.WithPublishGate(gate)

	if _, _, err := svc.Sessions(context.Background(), ctype.NewID(1), ctype.NewID(1), false, domain.Page{}); err != nil {
		t.Fatalf("被禁言者读会话列表不该失败: %v", err)
	}
	if len(gate.called) != 0 {
		t.Fatalf("读面不该调用发送闸门，实际 %v", gate.called)
	}
}

func TestSendAllowedWhenNotMuted(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t, 2, domain.StrangerAllowed)
	svc.WithPublishGate(&fakePublishGate{})

	if _, err := svc.Send(context.Background(), ctype.NewID(1), ctype.NewID(2),
		domain.TypeText, "正常私信", ""); err != nil {
		t.Fatalf("闸门放行时发送应成功: %v", err)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条私信，实际 %d", len(repo.sent))
	}
}

// 编译期确认端口形状。
var _ domain.PublishGate = (*fakePublishGate)(nil)
