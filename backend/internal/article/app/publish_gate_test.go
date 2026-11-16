package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	userdomain "myblogx/internal/user/domain"
)

// 发布闸门（票 399 / ADR-0032 决策 8）：
//   - 被禁言者不能创建文章（草稿也算）与更新文章；
//   - 错误值原样向上返回（user.muted + 到期时间文案）——article 不再翻译一次；
//   - 闸门放行时行为与改造前完全一致（正常作者照常发文）。

// fakePublishGate 记录被问询的用户，并按用例要求返回固定错误。
type fakePublishGate struct {
	err    error
	called []domain.UserID
}

func (g *fakePublishGate) EnsureCanPublish(_ context.Context, id domain.UserID) error {
	g.called = append(g.called, id)
	return g.err
}

// newGatedService 构造一个接了发布闸门的文章服务。
func newGatedService(t *testing.T, gate app.PublishGate) (*app.Service, *apptest.Repos) {
	t.Helper()
	svc, repos, _ := newService(t)
	return svc.WithPublishGate(gate), repos
}

// newMuteError 造一个"禁言到 baseTime+days"的真实错误值（user 模块的形状）。
func newMuteError(days int) error {
	return userdomain.NewMuteError(apptest.DefaultTime.Add(time.Duration(days)*24*time.Hour), apptest.DefaultTime)
}

// TestCreateRejectsMutedAuthor 被禁言者不能创建文章（草稿也算）。
func TestCreateRejectsMutedAuthor(t *testing.T) {
	t.Parallel()

	gate := &fakePublishGate{err: newMuteError(7)}
	svc, repos := newGatedService(t, gate)
	ctx := context.Background()

	for _, target := range []domain.Status{domain.StatusDraft, domain.StatusExamining} {
		_, err := svc.Create(ctx, app.CreateArticleCmd{
			Actor:        authorViewer(),
			Policy:       domain.OpenWritePolicy(),
			Title:        "禁言期间的发文",
			Content:      "正文",
			TargetStatus: target,
		})
		if !apperr.IsCode(err, apperr.CodeUserMuted) {
			t.Fatalf("目标状态 %s：应返回 user.muted，实际 %v", target, err)
		}
	}
	if len(gate.called) != 2 || gate.called[0] != author {
		t.Fatalf("闸门应对作者问询，实际 %v", gate.called)
	}
	if len(repos.FakeArticles().IDs()) != 0 {
		t.Fatal("被拒的文章不该落库")
	}
	if len(repos.FakeOutbox().Names()) != 0 {
		t.Fatalf("被拒的文章不该发事件: %v", repos.FakeOutbox().Names())
	}
}

// TestUpdateRejectsMutedAuthor 被禁言者不能更新文章。
func TestUpdateRejectsMutedAuthor(t *testing.T) {
	t.Parallel()

	gate := &fakePublishGate{err: newMuteError(1)}
	svc, repos := newGatedService(t, gate)
	seedArticle(t, repos, domain.ArticleID("3001"), author, domain.StatusDraft)
	ctx := context.Background()

	title := "改个标题"
	_, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: domain.ArticleID("3001"),
		Title:     &title,
		Policy:    domain.OpenWritePolicy(),
	})
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("应返回 user.muted，实际 %v", err)
	}
}

// TestPublishGateAllowsNormalAuthor 闸门放行时行为不变。
func TestPublishGateAllowsNormalAuthor(t *testing.T) {
	t.Parallel()

	gate := &fakePublishGate{}
	svc, _ := newGatedService(t, gate)
	ctx := context.Background()

	if _, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "正常发文",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
	}); err != nil {
		t.Fatalf("闸门放行时发文应成功: %v", err)
	}
	if len(gate.called) != 1 {
		t.Fatalf("闸门应被问询一次，实际 %d 次", len(gate.called))
	}
}
