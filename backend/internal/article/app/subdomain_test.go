package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
)

// ── 分类：创作页内联创建的用例层行为（#127）──
//
// 这组用例把"作者在写作页新建分类"需要后端保证的四件事钉在服务层：
// 归属当前作者、同名可读报错、空标题/纯空白拒绝、只允许登录用户，
// 以及新建的分类能立刻用于创建文章。
//
// 重名走的是 taxonomy.ErrTitleDuplicated（apperr.CodeTaxonomyTitleExists），
// 也就是 handler 会原样返回的 "名称重复" —— 前端只需要把这条可读文案展示出来，
// 而不是收到 MySQL 1062。

// newAuthorCategory 构造一个属于指定用户的分类，便于预置数据。
func newAuthorCategory(id domain.CategoryID, userID domain.UserID, title string) *taxonomy.Category {
	return taxonomy.RestoreCategory(id, userID, title)
}

func TestSaveCategoryTrimsTitleAndAttachesToViewer(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()

	saved, err := svc.SaveCategory(ctx, authorViewer(), "", "  前端  ")
	if err != nil {
		t.Fatalf("新建分类失败: %v", err)
	}
	if saved.Title() != "前端" {
		t.Fatalf("标题应去掉首尾空白，实际 %q", saved.Title())
	}
	if saved.UserID() != author {
		t.Fatalf("分类应归属当前作者 %s，实际 %s", author, saved.UserID())
	}
	if tx.Commits() != 1 {
		t.Fatalf("创建分类应在一次事务内完成，实际提交 %d 次", tx.Commits())
	}

	list, err := svc.ListCategories(ctx, authorViewer(), "")
	if err != nil {
		t.Fatalf("列出分类失败: %v", err)
	}
	if len(list) != 1 || list[0].ID() != saved.ID() {
		t.Fatalf("作者分类列表应只含刚建的那条，实际 %d 条", len(list))
	}
	// 预置的空 fake 仓储不应被无关写入影响。
	if calls := repos.FakeCategories().Calls("Save"); calls != 1 {
		t.Fatalf("应只保存一次，实际 %d 次", calls)
	}
}

func TestSaveCategoryDuplicateTitleReturnsReadableBusinessError(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	repos.FakeCategories().Seed(newAuthorCategory(domain.CategoryID(ctype.NewID(1)), author, "前端"))

	_, err := svc.SaveCategory(ctx, authorViewer(), "", "前端")
	if err == nil {
		t.Fatal("同用户同名分类应被拒绝，实际创建成功")
	}
	if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
		t.Fatalf("应返回领域错误 taxonomy.ErrTitleDuplicated，实际 %v", err)
	}
	if !apperr.IsCode(err, apperr.CodeTaxonomyTitleExists) {
		t.Fatalf("应带可读的 apperr 错误码，实际 %v", err)
	}
	if err.Error() != "名称重复" {
		t.Fatalf("面向用户的文案应为「名称重复」，实际 %q", err.Error())
	}
	// 是"查重后拒绝"，不是把仓储里的唯一键异常透传出去。
	if got := repos.FakeCategories().Calls("Save"); got != 0 {
		t.Fatalf("重名不应写库，实际 Save 被调用 %d 次", got)
	}
	if tx.Commits() != 0 || tx.Rollbacks() != 1 {
		t.Fatalf("重名应回滚事务，实际 commits=%d rollbacks=%d", tx.Commits(), tx.Rollbacks())
	}
}

func TestSaveCategoryRejectsBlankTitle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	for _, title := range []string{"", "   ", "\t\n"} {
		_, err := svc.SaveCategory(ctx, authorViewer(), "", title)
		if !errors.Is(err, taxonomy.ErrTitleRequired) {
			t.Fatalf("标题 %q 应被拒绝并返回 ErrTitleRequired，实际 %v", title, err)
		}
	}
	if got := repos.FakeCategories().Calls("Save"); got != 0 {
		t.Fatalf("空标题不应写库，实际 Save 被调用 %d 次", got)
	}
}

func TestSaveCategoryRequiresLogin(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.SaveCategory(ctx, domain.Anonymous(), "", "前端"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("匿名用户应被拒绝，实际 %v", err)
	}
}

func TestSaveCategoryTitleUniquePerAuthor(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.SaveCategory(ctx, authorViewer(), "", "技术"); err != nil {
		t.Fatalf("作者新建分类失败: %v", err)
	}
	// 分类是按用户唯一的：另一名作者用同一个标题应当成功。
	saved, err := svc.SaveCategory(ctx, strangerViewer(), "", "技术")
	if err != nil {
		t.Fatalf("另一作者同名分类不应冲突，实际 %v", err)
	}
	if saved.UserID() != stranger {
		t.Fatalf("分类应归属第二作者，实际 %s", saved.UserID())
	}
}

// ── 审核任务：驳回必须选一条固定原因 ──
//
// 驳回是"把一句话送到作者手里"这个动作：没有那句话，作者只收到"审核失败"、
// 不知道哪里不行。所以"必须选一条枚举"在用例层把关，而且不许留下任何副作用——
// 已经改了一半的文章状态 + 一条空原因通知，正是 #303 修过的那个缺陷。

func TestHandleReviewTaskRejectRequiresEnum(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1), author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	cases := []struct {
		name     string
		code     review.ReasonCode
		note     string
		wantCode apperr.Code
	}{
		// 没选：自由文本写得再长也不能代替枚举——前端要按值渲染，值才是结论。
		{name: "没选枚举", code: "", note: "内容不完整", wantCode: apperr.CodeArticleReviewReasonInvalid},
		// 选了一个词表之外的值：旧客户端 / 手搓请求，不能当枚举存下去。
		{name: "词表之外的值", code: "too_short", note: "", wantCode: apperr.CodeArticleReviewReasonInvalid},
		// 「其他原因」不补说明，作者收到的就是一句「其他原因」，等于没说。
		{name: "其他原因不补说明", code: review.ReasonOther, note: "   ", wantCode: apperr.CodeArticleReviewReasonRequired},
		// 说明超长会撞 varchar(500)：在进事务之前给出可读错误码。
		{name: "说明超长", code: review.ReasonQuality, note: strings.Repeat("长", review.ReasonNoteMaxLen+1), wantCode: apperr.CodeArticleReviewReasonTooLong},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(1), false, c.code, c.note)
			if got, ok := apperr.CodeOf(err); !ok || got != c.wantCode {
				t.Fatalf("应返回错误码 %s，实际 code=%s ok=%v err=%v", c.wantCode, got, ok, err)
			}
		})
	}

	if got, _ := repos.FakeArticles().Find(ctx, ctype.NewID(1)); got.Status() != domain.StatusExamining {
		t.Fatalf("被拒的驳回不应改文章状态，实际 %s", got.Status())
	}
	task, _ := repos.FakeReviews().Find(ctx, 1)
	if task.Status() != review.StatusPending {
		t.Fatalf("被拒的驳回不应动任务状态，实际 %s", task.Status())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("被拒的驳回不应发事件，实际 %v", names)
	}
}

// TestHandleReviewTaskRejectStoresEnumAndNote 合法驳回把"枚举 + 补充说明"落到任务上，
// 且补充说明去掉首尾空白：作者看到的那句话不该带着审核员手滑打出的换行。
func TestHandleReviewTaskRejectStoresEnumAndNote(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1), author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	if _, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(1), false, review.ReasonIncomplete, "  缺少示例代码\n"); err != nil {
		t.Fatalf("带原因的驳回失败: %v", err)
	}

	task, _ := repos.FakeReviews().Find(ctx, 1)
	if task.ReasonCode() != review.ReasonIncomplete {
		t.Fatalf("审核任务应记下枚举 %q，实际 %q", review.ReasonIncomplete, task.ReasonCode())
	}
	if task.ReasonNote() != "缺少示例代码" {
		t.Fatalf("审核任务应记下补充说明 %q，实际 %q", "缺少示例代码", task.ReasonNote())
	}
	if task.Status() != review.StatusRejected {
		t.Fatalf("任务应为已驳回，实际 %s", task.Status())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticleRejected {
		t.Fatalf("应发一条驳回事件，实际 %v", names)
	}
}

// TestHandleReviewTaskRejectOnlyEnumIsEnough 只选一条、不补说明是合法输入：
// 自由文本是"可选"，别把它变成第二个必填项。
func TestHandleReviewTaskRejectOnlyEnumIsEnough(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1), author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	if _, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(1), false, review.ReasonQuality, ""); err != nil {
		t.Fatalf("只选枚举的驳回应成功，实际 %v", err)
	}
	task, _ := repos.FakeReviews().Find(ctx, 1)
	if task.ReasonNote() != "" {
		t.Fatalf("没补说明时不该凭空造出一句，实际 %q", task.ReasonNote())
	}
	if task.ReasonCode() != review.ReasonQuality {
		t.Fatalf("枚举应落在任务上，实际 %q", task.ReasonCode())
	}
}

// TestHandleReviewTaskApproveNeedsNoReason 通过不需要原因：
// "驳回要选原因"这条规则不能被顺手加到通过上，否则管理员点"通过"也要先写理由。
func TestHandleReviewTaskApproveNeedsNoReason(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1), author, domain.StatusExamining)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	// 通过时即便带了原因也一律丢弃：通过的语义里没有"驳回原因"这一项，
	// 留下它只会让队列里出现一条状态是 approved 却带 reason 的记录。
	if _, err := svc.HandleReviewTask(ctx, adminViewer(), ctype.NewID(1), true, review.ReasonOther, "不该被记录"); err != nil {
		t.Fatalf("审核通过不该要求原因，实际 %v", err)
	}

	got, _ := repos.FakeArticles().Find(ctx, ctype.NewID(1))
	if got.Status() != domain.StatusPublished {
		t.Fatalf("通过后应为已发布，实际 %s", got.Status())
	}
	task, _ := repos.FakeReviews().Find(ctx, 1)
	if task.Status() != review.StatusApproved {
		t.Fatalf("任务应为已通过，实际 %s", task.Status())
	}
	if task.Reason() != "" {
		t.Fatalf("通过不该在任务上留下原因，实际 %q", task.Reason())
	}
}

// ── 审核队列：条目带正文、空队列不是错误 ──
//
// 审核员要在列表里直接判断一篇文章，只有标题远远不够：标题看不出东拼西凑、
// 通篇 AI 腔或整段复制。所以队列条目必须带正文（#307）。
//
// 正文取的是**文章当前内容**，不是提交时的快照：审核任务表没有正文列
// （零 DDL 硬约束），提交时也只快照了标题与作者。这个取舍写在
// ListReviewTasks 的注释里，并由下面第一条用例锁住。

func TestListReviewTasksIncludesCurrentArticleBody(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := apptest.NewArticle().
		ID(ctype.NewID(1)).
		Author(author).
		Status(domain.StatusExamining).
		Title("待审稿").
		Content("<p>第一段</p><p>第二段</p>").
		Build()
	repos.FakeArticles().Seed(a)
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, a.ID(), author, review.StatusPending))

	views, total, err := svc.ListReviewTasks(ctx, adminViewer(), nil, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("列出审核任务失败: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("应有 1 条任务，实际 total=%d len=%d", total, len(views))
	}
	if views[0].Content != a.Content() {
		t.Fatalf("条目正文应为文章当前内容 %q，实际 %q", a.Content(), views[0].Content)
	}
}

func TestListReviewTasksEmptyQueueIsNotAnError(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	views, total, err := svc.ListReviewTasks(ctx, adminViewer(), nil, domain.Page{})
	if err != nil {
		t.Fatalf("空队列是正常读面，不应报错: %v", err)
	}
	if total != 0 || len(views) != 0 {
		t.Fatalf("空队列应返回空列表，实际 total=%d len=%d", total, len(views))
	}
}

// TestListReviewTasksToleratesMissingArticle 任务行指向的文章已被删除时，
// 整页队列不能因此失败：条目照常出现、正文留空，审核员仍能按标题处理掉它。
func TestListReviewTasksToleratesMissingArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	// 只预置任务，不预置文章。
	repos.FakeReviews().Seed(apptest.NewTaskForTest(1, ctype.NewID(404), author, review.StatusPending))

	views, total, err := svc.ListReviewTasks(ctx, adminViewer(), nil, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("文章已删不应让队列失败: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("任务仍应出现，实际 total=%d len=%d", total, len(views))
	}
	if views[0].Content != "" {
		t.Fatalf("文章不存在时正文应为空，实际 %q", views[0].Content)
	}
}

func TestCreateArticleWithFreshlyCreatedCategory(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	saved, err := svc.SaveCategory(ctx, authorViewer(), "", "新分类")
	if err != nil {
		t.Fatalf("新建分类失败: %v", err)
	}
	categoryID := saved.ID()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "用新分类的文章",
		Content:      "正文",
		CategoryID:   &categoryID,
		TargetStatus: domain.StatusDraft,
	})
	if err != nil {
		t.Fatalf("用刚建分类建文章失败: %v", err)
	}
	if a.CategoryID() == nil || *a.CategoryID() != categoryID {
		t.Fatalf("文章分类应为新建分类 %s，实际 %v", categoryID, a.CategoryID())
	}
}
