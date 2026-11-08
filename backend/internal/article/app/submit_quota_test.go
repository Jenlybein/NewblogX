package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 投稿配额（票 304 / ADR-0031）在用例层的可观察行为。
//
// 只断言"文章最终落在哪个状态、返回哪个错误码、额度有没有被归还"，
// 不进 Redis、也不断言适配器内部动作（spec 的 Testing Decisions）。

// newQuotaService 构造带投稿配额 fake 的用例服务。
func newQuotaService(t *testing.T, dailyMax int) (*app.Service, *apptest.Repos, *apptest.Tx, *apptest.SubmitQuota) {
	t.Helper()
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	quota := apptest.NewSubmitQuota(dailyMax)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), quota, nil, nil, nil)
	repos.FakeUsers().Seed(app.AuthorView{ID: author, Nickname: "作者", Username: "u100"})
	repos.FakeCategories().Seed()
	return svc, repos, tx, quota
}

// submitCmd 是一次"提交审核"的新建请求。
func submitCmd() app.CreateArticleCmd {
	return app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "要提交的文章",
		Content:      "正文",
		TargetStatus: domain.StatusExamining,
	}
}

func TestCreateSubmitConsumesOneQuotaAndDraftDoesNot(t *testing.T) {
	t.Parallel()

	svc, _, _, quota := newQuotaService(t, 15)

	if _, err := svc.Create(context.Background(), submitCmd()); err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	if got := quota.Used(author); got != 1 {
		t.Fatalf("一次提交动作应计 1 次额度，实际 %d", got)
	}

	// 存草稿不是提交动作：不占额度（计量单位是提交，不是"写过文章"）。
	draft := submitCmd()
	draft.TargetStatus = domain.StatusDraft
	if _, err := svc.Create(context.Background(), draft); err != nil {
		t.Fatalf("存草稿失败: %v", err)
	}
	if got := quota.Used(author); got != 1 {
		t.Fatalf("草稿不应占用额度，实际 %d", got)
	}
}

func TestCreateSubmitRejectedWhenQuotaExhausted(t *testing.T) {
	t.Parallel()

	svc, repos, tx, quota := newQuotaService(t, 15)
	quota.Seed(author, 15)

	_, err := svc.Create(context.Background(), submitCmd())
	if !apperr.IsCode(err, apperr.CodeArticleSubmitQuotaExceeded) {
		t.Fatalf("超额应返回可区分的配额错误码，实际 %v", err)
	}
	// 超限时一个字节都不该落库。
	if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
		t.Fatalf("超限不应写入文章，实际 %v", ids)
	}
	if tx.Commits() != 0 {
		t.Fatalf("超限不应提交事务，实际 %d 次", tx.Commits())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("超限不应发事件，实际 %v", names)
	}
}

func TestCreateSubmitReturnsQuotaOnSystemFailure(t *testing.T) {
	t.Parallel()

	svc, _, tx, quota := newQuotaService(t, 15)
	tx.FailNext(errors.New("db down"))

	if _, err := svc.Create(context.Background(), submitCmd()); err == nil {
		t.Fatal("系统故障时应返回错误")
	}
	// 因系统故障失败的提交不占用额度（spec 决策 18）。
	if got := quota.Used(author); got != 0 {
		t.Fatalf("失败提交不应占用额度，实际 %d", got)
	}
}

func TestCreateSubmitAdminIsNotQuotaLimited(t *testing.T) {
	t.Parallel()

	svc, _, _, quota := newQuotaService(t, 15)
	quota.Seed(author, 15)

	cmd := submitCmd()
	cmd.Actor = adminViewer()
	if _, err := svc.Create(context.Background(), cmd); err != nil {
		t.Fatalf("管理员不应受投稿配限制: %v", err)
	}
	if got := quota.Used(admin); got != 0 {
		t.Fatalf("管理员提交不应计额度，实际 %d", got)
	}
}

func TestUpdateResubmitConsumesQuotaButExplicitDraftDoesNot(t *testing.T) {
	t.Parallel()

	svc, repos, _, quota := newQuotaService(t, 15)
	articleID := ctype.NewID(1001)
	seedArticle(t, repos, articleID, author, domain.StatusDraft)

	title := "提交审核"
	target := domain.StatusExamining
	if _, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		Actor:         authorViewer(),
		ArticleID:     articleID,
		Policy:        domain.OpenWritePolicy(),
		TargetStatus:  &target,
		Title:         &title,
		SkipExamining: false,
	}); err != nil {
		t.Fatalf("提交审核失败: %v", err)
	}
	if got := quota.Used(author); got != 1 {
		t.Fatalf("更新路径的一次提交也应计额度，实际 %d", got)
	}

	// 退回草稿不是提交动作。
	back := domain.StatusDraft
	if _, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		Actor:        authorViewer(),
		ArticleID:    articleID,
		Policy:       domain.OpenWritePolicy(),
		TargetStatus: &back,
	}); err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if got := quota.Used(author); got != 1 {
		t.Fatalf("退回草稿不应消耗额度，实际 %d", got)
	}
}

func TestUpdateResubmitRejectedWhenQuotaExhausted(t *testing.T) {
	t.Parallel()

	svc, repos, _, quota := newQuotaService(t, 15)
	articleID := ctype.NewID(1001)
	seedArticle(t, repos, articleID, author, domain.StatusDraft)
	quota.Seed(author, 15)

	title := "提交审核"
	target := domain.StatusExamining
	_, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		Actor:        authorViewer(),
		ArticleID:    articleID,
		Policy:       domain.OpenWritePolicy(),
		TargetStatus: &target,
		Title:        &title,
	})
	if !apperr.IsCode(err, apperr.CodeArticleSubmitQuotaExceeded) {
		t.Fatalf("超额应返回可区分的配额错误码，实际 %v", err)
	}
	// 文章不该被改动（事务回滚）：标题保持原值、状态仍是草稿。
	stored, ferr := repos.FakeArticles().Find(context.Background(), articleID)
	if ferr != nil {
		t.Fatalf("读取文章失败: %v", ferr)
	}
	if stored.Title() != "测试文章" || stored.Status() != domain.StatusDraft {
		t.Fatalf("超限时不应改动文章，实际标题=%q 状态=%s", stored.Title(), stored.Status())
	}
}

func TestUpdateResubmitReturnsQuotaOnSystemFailure(t *testing.T) {
	t.Parallel()

	svc, repos, _, quota := newQuotaService(t, 15)
	articleID := ctype.NewID(1001)
	seedArticle(t, repos, articleID, author, domain.StatusDraft)
	// 故障注入在"落库"这一步而不是事务入口：预占已经发生，归还路径才真的被走到。
	repos.FakeArticles().FailNext("Update", errors.New("db down"))

	title := "提交审核"
	target := domain.StatusExamining
	if _, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		Actor:        authorViewer(),
		ArticleID:    articleID,
		Policy:       domain.OpenWritePolicy(),
		TargetStatus: &target,
		Title:        &title,
	}); err == nil {
		t.Fatal("系统故障时应返回错误")
	}
	if got := quota.Used(author); got != 0 {
		t.Fatalf("失败提交不应占用额度，实际 %d", got)
	}
}
