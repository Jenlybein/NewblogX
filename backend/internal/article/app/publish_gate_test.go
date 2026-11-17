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

// TestCreateRejectsMutedAuthorOnSubmit 被禁言者**提交审核**被拒、**存草稿**放行。
//
// 口径是"只拦发布类动作"（ADR-0032 决策 8、10）：草稿不进公开面、也不占投稿配额，
// 拦它比文档更严且没有任何收益 —— 被禁言的人连整理草稿都做不到。
func TestCreateRejectsMutedAuthorOnSubmit(t *testing.T) {
	t.Parallel()

	gate := &fakePublishGate{err: newMuteError(7)}
	svc, repos := newGatedService(t, gate)
	ctx := context.Background()

	// 提交审核：拒绝，且闸门问询一次。
	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "禁言期间的投稿",
		Content:      "正文",
		TargetStatus: domain.StatusExamining,
	})
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("提交审核应返回 user.muted，实际 %v", err)
	}
	if len(gate.called) != 1 || gate.called[0] != author {
		t.Fatalf("闸门应对作者问询一次，实际 %v", gate.called)
	}
	if len(repos.FakeArticles().IDs()) != 0 {
		t.Fatal("被拒的文章不该落库")
	}
	if len(repos.FakeOutbox().Names()) != 0 {
		t.Fatalf("被拒的文章不该发事件: %v", repos.FakeOutbox().Names())
	}

	// 存草稿：放行，且**不问闸门**（问一次就说明还在无条件拦）。
	if _, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "禁言期间的草稿",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
	}); err != nil {
		t.Fatalf("存草稿不该被禁言拦住: %v", err)
	}
	if len(gate.called) != 1 {
		t.Fatalf("存草稿不该问询闸门，实际问询 %d 次", len(gate.called))
	}
}

// TestUpdateRejectsMutedAuthorOnlyOnSubmit 被禁言者改草稿放行、把草稿提交被拒。
//
// 与 Create 同一口径：闸门挂在"这次更新真的会提交"（preview.NeedsSubmit）上，
// 而不是挂在"调了 Update"上。
func TestUpdateRejectsMutedAuthorOnlyOnSubmit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 场景一：改草稿（不提交）——放行，且不问闸门。
	gate := &fakePublishGate{err: newMuteError(1)}
	svc, repos := newGatedService(t, gate)
	seedArticle(t, repos, domain.ArticleID("3001"), author, domain.StatusDraft)
	title := "改个标题"
	if _, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: domain.ArticleID("3001"),
		Title:     &title,
		Policy:    domain.OpenWritePolicy(),
	}); err != nil {
		t.Fatalf("改草稿不该被禁言拦住: %v", err)
	}
	if len(gate.called) != 0 {
		t.Fatalf("改草稿不该问询闸门，实际 %d 次", len(gate.called))
	}

	// 场景二：把草稿提交审核——拒绝。
	gate2 := &fakePublishGate{err: newMuteError(1)}
	svc2, repos2 := newGatedService(t, gate2)
	seedArticle(t, repos2, domain.ArticleID("3002"), author, domain.StatusDraft)
	submit := domain.StatusExamining
	_, err := svc2.Update(ctx, app.UpdateArticleCmd{
		Actor:         authorViewer(),
		ArticleID:     domain.ArticleID("3002"),
		TargetStatus:  &submit,
		Policy:        domain.OpenWritePolicy(),
	})
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("提交审核应返回 user.muted，实际 %v", err)
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
		TargetStatus: domain.StatusExamining,
	}); err != nil {
		t.Fatalf("闸门放行时发文应成功: %v", err)
	}
	if len(gate.called) != 1 {
		t.Fatalf("提交审核时闸门应被问询一次，实际 %d 次", len(gate.called))
	}
}
