package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
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
