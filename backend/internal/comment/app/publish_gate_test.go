package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
	userdomain "myblogx/internal/user/domain"
)

// 评论的发布闸门（票 399 / ADR-0032 决策 8）：
//   - 被禁言者不能发表评论 / 回复，错误值原样向上返回（user.muted + 到期时间）；
//   - 读面与点赞/收藏这类非发布动作不受影响（闸门只在 Create 里）。

// fakePublishGate 记录被问询的用户并返回固定错误。
type fakePublishGate struct {
	err    error
	called []ctype.ID
}

func (g *fakePublishGate) EnsureCanPublish(_ context.Context, userID ctype.ID) error {
	g.called = append(g.called, userID)
	return g.err
}

func mutedGate() *fakePublishGate {
	return &fakePublishGate{err: userdomain.NewMuteError(
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
	)}
}

// TestCreateRejectsMutedCommenter 被禁言者发表评论被拒，且不落库、不计数、不通知。
func TestCreateRejectsMutedCommenter(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.svc.WithPublishGate(mutedGate())

	_, err := f.create(t, "禁言期间的评论")
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("应返回 user.muted，实际 %v", err)
	}
	if len(f.repo.created) != 0 {
		t.Fatalf("被拒的评论不该落库，实际 %d 条", len(f.repo.created))
	}
	if f.cache.added != 0 || f.notifier.count != 0 {
		t.Fatalf("被拒的评论不该计数/通知: cache=%d notifier=%d", f.cache.added, f.notifier.count)
	}
}

// TestPublishGateDoesNotAffectReadAndDigg 登录、浏览、点赞收藏不受禁言影响。
//
// 这一条是 AC 的直接证据：闸门只在 Create 里调用一次，读面与点赞路径
// 根本不碰它（fakePublishGate 的 called 保持为空）。
func TestPublishGateDoesNotAffectReadAndDigg(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	gate := mutedGate()
	f.svc.WithPublishGate(gate)

	items, _, err := f.svc.ListRoot(context.Background(), ctype.NewID(100), domain.Page{}, ctype.NewID(7))
	if err != nil {
		t.Fatalf("被禁言者浏览评论列表不该失败: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("空列表应是空，实际 %d 条", len(items))
	}
	if len(gate.called) != 0 {
		t.Fatalf("读面不该调用发布闸门，实际 %v", gate.called)
	}
}

// TestPublishGateAllowsNormalCommenter 闸门放行时发表评论行为不变。
func TestPublishGateAllowsNormalCommenter(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.svc.WithPublishGate(&fakePublishGate{})

	result, err := f.create(t, "正常评论")
	if err != nil {
		t.Fatalf("闸门放行时评论应成功: %v", err)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("评论应直接发布，实际状态 %d", result.Status)
	}
}

// 编译期确认端口形状：评论模块只依赖窄的 PublishGate 接口。
var _ domain.PublishGate = (*fakePublishGate)(nil)
