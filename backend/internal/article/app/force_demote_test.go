package app_test

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
	article_review_enum "myblogx/internal/kernel/enum/article_review_enum"
)

// 本文件锁定「管理员强制退回草稿」这条运营纠错用例。
//
// 三件事必须一起成立，否则工具就是半成品：
//  1. 已公开可见的文章退回后真的不可见（状态复位 + 审核痕迹清空）；
//  2. 已经是草稿时按"目的已达成"返回成功，且不写库（连点两次不留痕迹）；
//  3. 原因必填 —— 这句话是要发给作者的，空串必须是一个明确的参数错误码。

// fakeForceDemoteNotifier 记录端口收到的通知，用来断言"作者到底收到了什么"。
type fakeForceDemoteNotifier struct {
	notices []app.ForceDemoteNotice
	err     error
}

func (f *fakeForceDemoteNotifier) NotifyForceDemoted(_ context.Context, n app.ForceDemoteNotice) error {
	if f.err != nil {
		return f.err
	}
	f.notices = append(f.notices, n)
	return nil
}

// seedVisiblePublishedArticle 预置一篇已发布文章（默认公开可见）。
func seedVisiblePublishedArticle(t *testing.T, repos *apptest.Repos, id uint64, visibility domain.Visibility) domain.ArticleID {
	t.Helper()
	articleID := domain.ArticleID(ctype.NewID(id))
	a := apptest.NewArticle().
		ID(articleID).
		Author(author).
		Status(domain.StatusPublished).
		Visibility(visibility).
		Title("被误放行的文章").
		SubmittedAt(apptest.DefaultTime).
		Build()
	repos.FakeArticles().Seed(a)
	return articleID
}

// TestForceDemotePublishedArticleBecomesInvisibleDraft 是最核心的一条：
// 一篇文章已经在公网上可见，强制退回后必须是草稿、对匿名访客不可见、
// 待处理任务作废、留一条能追到执行人的档案，并且作者收到一条带原因的通知。
func TestForceDemotePublishedArticleBecomesInvisibleDraft(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	notifier := &fakeForceDemoteNotifier{}
	svc.WithNotifier(notifier)

	ctx := context.Background()
	articleID := seedVisiblePublishedArticle(t, repos, 9001, domain.VisibilityVisible)
	// 一篇"已发布但仍在等模型判定"的文章可能还挂着一审任务，退回时必须一起作废。
	repos.FakeReviews().Seed(apptest.NewTaskForTest(5001, articleID, author, review.StatusPending))

	const reason = "评分 6 分，未达到发布线，误放行了"
	if err := svc.ForceDemote(ctx, articleID, admin, reason); err != nil {
		t.Fatalf("强制退回失败: %v", err)
	}

	stored, err := repos.Articles().Find(ctx, articleID)
	if err != nil {
		t.Fatalf("读取文章失败: %v", err)
	}
	if stored.Status() != domain.StatusDraft {
		t.Fatalf("状态应为草稿，实际 %s", stored.Status())
	}
	if stored.IsPubliclyVisible() {
		t.Fatal("退回后不该再对匿名访客可见")
	}
	if stored.SubmittedAt() != nil || stored.ReviewedAt() != nil || stored.ReviewedBy() != nil {
		t.Fatalf("审核痕迹应清空: submitted=%v reviewed=%v by=%v",
			stored.SubmittedAt(), stored.ReviewedAt(), stored.ReviewedBy())
	}

	// 待处理任务作废：文章已经不在发布链路上，审核队列里不该留一条"审了也没用"的任务。
	tasks, err := repos.FakeReviews().ListAll(ctx, nil)
	if err != nil {
		t.Fatalf("读取审核任务失败: %v", err)
	}
	var canceled, trace *review.Task
	for _, task := range tasks {
		switch {
		case task.ID() == 5001:
			canceled = task
		case task.Source() == review.Source(article_review_enum.SourceForceDemote):
			trace = task
		}
	}
	if canceled == nil || canceled.Status() != review.StatusCanceled {
		t.Fatalf("原有待处理任务应被撤销，实际 %+v", canceled)
	}
	if !strings.Contains(canceled.Reason(), reason) {
		t.Fatalf("撤销原因里应带上管理员填的原因，实际 %q", canceled.Reason())
	}

	// 档案：一条创建即已撤销的任务行，来源 force_demote，能追到人、原来是什么状态。
	if trace == nil {
		t.Fatal("应留一条 force_demote 来源的审核任务档案")
	}
	if trace.Status() != review.StatusCanceled {
		t.Fatalf("档案行应是已撤销（不进待办队列），实际 %s", trace.Status())
	}
	if trace.ReviewedBy() == nil || *trace.ReviewedBy() != admin {
		t.Fatalf("档案行必须记下执行人，实际 %v", trace.ReviewedBy())
	}
	if trace.ReviewedAt() == nil {
		t.Fatal("档案行必须记下处置时间")
	}
	if trace.PublishStatus() != domain.StatusPublished {
		t.Fatalf("档案行应记下退回前的状态（已发布），实际 %s", trace.PublishStatus())
	}
	if !strings.Contains(trace.Reason(), reason) {
		t.Fatalf("档案行应记下原因，实际 %q", trace.Reason())
	}

	if len(notifier.notices) != 1 {
		t.Fatalf("应给作者发一条站内信，实际 %d 条", len(notifier.notices))
	}
	got := notifier.notices[0]
	if got.AuthorID != author || got.ArticleID != articleID || got.OperatorID != admin {
		t.Fatalf("通知收/发件人与文章不对: %+v", got)
	}
	if got.Reason != reason || got.Title != "被误放行的文章" {
		t.Fatalf("通知正文素材不对: %+v", got)
	}
}

// TestForceDemoteAlreadyDraftIsNoop 钉住幂等：目的已达成时不写库、不留第二份档案、
// 也不重复骚扰作者（管理后台连点两次、客户端自动重试都会走到这里）。
func TestForceDemoteAlreadyDraftIsNoop(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	notifier := &fakeForceDemoteNotifier{}
	svc.WithNotifier(notifier)

	ctx := context.Background()
	seedArticle(t, repos, ctype.NewID(9002), author, domain.StatusDraft)

	if err := svc.ForceDemote(ctx, ctype.NewID(9002), admin, "已经是草稿了"); err != nil {
		t.Fatalf("已是草稿不该报错（目的已达成）: %v", err)
	}
	if calls := repos.FakeArticles().Calls("Update"); calls != 0 {
		t.Fatalf("已是草稿时不该写文章表，实际 Update %d 次", calls)
	}
	if calls := repos.FakeReviews().Calls("Save"); calls != 0 {
		t.Fatalf("已是草稿时不该新建任务档案，实际 Save %d 次", calls)
	}
	if len(notifier.notices) != 0 {
		t.Fatalf("什么都没变时不该发通知，实际 %d 条", len(notifier.notices))
	}
}

// TestForceDemoteRequiresReason 钉住"原因必填"：空串与纯空白都要拿到明确的
// 参数错误码，而不是静默落一条空原因（那句话是要发给作者的）。
func TestForceDemoteRequiresReason(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{"", " ", "\n\t "} {
		reason := reason
		t.Run("reason="+strings.ReplaceAll(reason, "\n", `\n`), func(t *testing.T) {
			t.Parallel()

			svc, repos, _ := newService(t)
			notifier := &fakeForceDemoteNotifier{}
			svc.WithNotifier(notifier)

			ctx := context.Background()
			articleID := seedVisiblePublishedArticle(t, repos, 9003, domain.VisibilityVisible)

			err := svc.ForceDemote(ctx, articleID, admin, reason)
			if !apperr.IsCode(err, apperr.CodeArticleForceDemoteReasonRequired) {
				t.Fatalf("应返回 force_demote_reason_required，实际 %v", err)
			}
			// 文案必须是可读的中文，不能退化成错误码本身。
			if msg := apperr.Message(apperr.CodeArticleForceDemoteReasonRequired); msg == "" ||
				msg == string(apperr.CodeArticleForceDemoteReasonRequired) {
				t.Fatalf("错误码没有中文文案: %q", msg)
			}
			if calls := repos.FakeArticles().Calls("Update"); calls != 0 {
				t.Fatalf("参数不合法时不该写库，实际 Update %d 次", calls)
			}
			if len(notifier.notices) != 0 {
				t.Fatalf("参数不合法时不该发通知，实际 %d 条", len(notifier.notices))
			}
		})
	}
}

// TestForceDemoteRejectsLongReason 钉住长度上限：reason 列是 varchar(500)，
// 枚举前缀之外只留 300 字符，超了要在用例层拦下，而不是写库时变成 500。
func TestForceDemoteRejectsLongReason(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	articleID := seedVisiblePublishedArticle(t, repos, 9004, domain.VisibilityVisible)

	err := svc.ForceDemote(ctx, articleID, admin, strings.Repeat("因", review.ReasonNoteMaxLen+1))
	if !apperr.IsCode(err, apperr.CodeArticleReviewReasonTooLong) {
		t.Fatalf("超长原因应返回 review_reason_too_long，实际 %v", err)
	}
}

// TestForceDemoteFromExamining 钉住另一个起点：还没放行、但已在发布链路上的文章
// 也能被撤回（作者不会卡在"审核中"这个既不公开也改不了的档位上）。
func TestForceDemoteFromExamining(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(9005))
	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(articleID).
		Author(author).
		Status(domain.StatusExamining).
		Build())

	if err := svc.ForceDemote(ctx, articleID, admin, "误进审核队列"); err != nil {
		t.Fatalf("审核中的文章应能退回草稿: %v", err)
	}
	stored, err := repos.Articles().Find(ctx, articleID)
	if err != nil {
		t.Fatalf("读取文章失败: %v", err)
	}
	if stored.Status() != domain.StatusDraft {
		t.Fatalf("状态应为草稿，实际 %s", stored.Status())
	}
}

// TestForceDemoteRequiresOperator 钉住"没有操作人就没有留痕"：
// 零值操作人必须被拦在写库之前（否则档案行会写进一个空的 reviewed_by）。
func TestForceDemoteRequiresOperator(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	articleID := seedVisiblePublishedArticle(t, repos, 9006, domain.VisibilityVisible)

	err := svc.ForceDemote(ctx, articleID, "", "缺少操作人")
	if !apperr.IsCode(err, apperr.CodeArticleForbidden) {
		t.Fatalf("缺少操作人应返回 article.forbidden，实际 %v", err)
	}
	if calls := repos.FakeArticles().Calls("Update"); calls != 0 {
		t.Fatalf("缺少操作人时不该写库，实际 Update %d 次", calls)
	}
}

// TestForceDemoteMissingArticleReportsNotFound 钉住不存在的文章：明确的 404 类
// 语义（article.not_found），不是"退回成功"。
func TestForceDemoteMissingArticleReportsNotFound(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	err := svc.ForceDemote(context.Background(), ctype.NewID(9999), admin, "不存在的文章")
	if !apperr.IsCode(err, apperr.CodeArticleNotFound) {
		t.Fatalf("应返回 article.not_found，实际 %v", err)
	}
}
