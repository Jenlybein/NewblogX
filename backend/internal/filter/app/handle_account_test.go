package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// 风险页的账号处置（票 399 / ADR-0032 决策 8）：
//   - mute 动作写账号禁言并把记录标成「账号已处置」，不删内容；
//   - ban 动作封禁账号，同样不删内容；
//   - 禁言天数原样交给账号端口（1–365 / 缺省 7 的校验在 user 模块，单一来源）；
//   - 账号端口没装配时失败关闭，记录保持待处理；
//   - 已处置的记录幂等，不再重复处罚账号。

// fakeAccountDisposer 记录封禁 / 禁言调用。
type fakeAccountDisposer struct {
	banned   []ctype.ID
	muted    []ctype.ID
	muteDays []int
	err      error
}

func (d *fakeAccountDisposer) BanUser(_ context.Context, _, userID ctype.ID) error {
	if d.err != nil {
		return d.err
	}
	d.banned = append(d.banned, userID)
	return nil
}

func (d *fakeAccountDisposer) MuteUser(_ context.Context, _, userID ctype.ID, days int) error {
	if d.err != nil {
		return d.err
	}
	d.muted = append(d.muted, userID)
	d.muteDays = append(d.muteDays, days)
	return nil
}

var _ domain.AccountDisposer = (*fakeAccountDisposer)(nil)

// newAccountHandleService 构造一个同时带内容处置与账号处置的服务。
func newAccountHandleService(t *testing.T, surface domain.Surface) (*app.Service, *handleRepo, *fakeDisposer, *fakeAccountDisposer) {
	t.Helper()
	repo := &handleRepo{
		row: domain.Hit{
			ID:        ctype.NewID(700),
			Surface:   surface,
			ContentID: ctype.NewID(42),
			UserID:    ctype.NewID(900),
			Status:    domain.StatusPending,
			Words:     []domain.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		},
		found:   true,
		updated: true,
	}
	content := &fakeDisposer{}
	account := &fakeAccountDisposer{}
	svc := app.New(nil, repo, &fakeLogger{}).
		WithDisposer(content).
		WithAccountDisposer(account)
	return svc, repo, content, account
}

func TestHandleMuteMarksAccountAndKeepsContent(t *testing.T) {
	t.Parallel()

	svc, repo, content, account := newAccountHandleService(t, domain.SurfaceArticle)
	got, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionMute,
		MuteDays:  30,
		Note:      "惯犯，先禁言",
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("禁言处置应成功: %v", err)
	}
	if len(account.muted) != 1 || account.muted[0].Uint64() != 900 {
		t.Fatalf("应对命中记录的作者执行禁言，实际 %v", account.muted)
	}
	if len(account.muteDays) != 1 || account.muteDays[0] != 30 {
		t.Fatalf("禁言天数应原样传给账号端口，实际 %v", account.muteDays)
	}
	if len(content.deletedArticles) != 0 {
		t.Fatalf("账号动作不该删内容，实际 %v", content.deletedArticles)
	}
	if got.Hit.Status != domain.StatusAccount {
		t.Fatalf("记录状态应为账号已处置（account），实际 %s", got.Hit.Status)
	}
	if got.Hit.HandlerID.Uint64() != 1 || got.Hit.Note != "惯犯，先禁言" {
		t.Fatalf("处置痕迹没写全: handler=%v note=%q", got.Hit.HandlerID, got.Hit.Note)
	}
}

func TestHandleMuteDefaultsDaysPassedAsZero(t *testing.T) {
	t.Parallel()

	svc, repo, _, account := newAccountHandleService(t, domain.SurfaceComment)
	if _, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionMute,
		HandlerID: ctype.NewID(1),
	}); err != nil {
		t.Fatalf("缺省天数的禁言应成功（默认 7 天由 user 模块补齐）: %v", err)
	}
	if len(account.muteDays) != 1 || account.muteDays[0] != 0 {
		t.Fatalf("缺省天数应原样传 0（缺省值只在 user 模块有一份），实际 %v", account.muteDays)
	}
}

func TestHandleBanBansAccountAndKeepsContent(t *testing.T) {
	t.Parallel()

	svc, repo, content, account := newAccountHandleService(t, domain.SurfaceChat)
	got, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionBan,
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("封禁处置应成功: %v", err)
	}
	if len(account.banned) != 1 || account.banned[0].Uint64() != 900 {
		t.Fatalf("应对命中记录的作者执行封禁，实际 %v", account.banned)
	}
	if len(content.deletedArticles) != 0 {
		t.Fatalf("封禁不该删内容，实际 %v", content.deletedArticles)
	}
	if got.Hit.Status != domain.StatusAccount {
		t.Fatalf("记录状态应为账号已处置，实际 %s", got.Hit.Status)
	}
}

func TestHandleAccountActionFailsClosedWithoutPort(t *testing.T) {
	t.Parallel()

	// 只接内容处置、不接账号处置：禁言必须报错，不能静默当成"已处理"。
	svc, repo, _ := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	_, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionMute,
		HandlerID: ctype.NewID(1),
	})
	if !apperr.IsCode(err, apperr.CodeFilterDisposeUnavailable) {
		t.Fatalf("账号端口未装配应返回 filter.dispose_unavailable，实际 %v", err)
	}
	if len(repo.marked) != 0 {
		t.Fatalf("账号处罚失败时不该把记录标成已处置，实际 %v", repo.marked)
	}
}

func TestHandleAccountActionIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repo, _, account := newAccountHandleService(t, domain.SurfaceArticle)
	repo.row.Status = domain.StatusAccount

	got, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionMute,
		MuteDays:  7,
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("重复处置应幂等返回当前状态: %v", err)
	}
	if !got.AlreadyHandled {
		t.Fatal("应标记为已处置")
	}
	if len(account.muted) != 0 {
		t.Fatalf("已处置的记录不该再重复禁言，实际 %v", account.muted)
	}
}

func TestHandleAccountActionReportsPortFailure(t *testing.T) {
	t.Parallel()

	svc, repo, _, account := newAccountHandleService(t, domain.SurfaceArticle)
	account.err = errors.New("账号模块不可用")
	if _, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionBan,
		HandlerID: ctype.NewID(1),
	}); err == nil {
		t.Fatal("账号处罚失败时应向上返回错误")
	}
	if len(repo.marked) != 0 {
		t.Fatalf("账号处罚失败时记录应保持待处理，实际 %v", repo.marked)
	}
}
