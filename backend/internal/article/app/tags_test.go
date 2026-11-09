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
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
)

// 标签是**全站词库**（没有 owner，仅管理员可写），后台标签管理页直接消费这两条规则：
//
//  1. 删除只对"没被任何文章引用"的标签开放。被引用就拒绝，并且要给出可读提示
//     （带篇数、建议改用停用），而不是让管理员自己去数关联表。
//  2. 编辑是读—改—写。没传的字段必须保持原值 —— 编辑一个停用标签不能把它
//     悄悄放回全站可选，没传排序/描述也不能把库里那两列清空。
//
// 引用数一律按关联表实时统计：tag_models.article_count 没有维护者，恒为 0，
// 按它做门禁会把"真的有 3 篇在用"的标签放行。

// seedTag 预置一个可控的标签（标题 / 排序 / 描述 / 启用状态）。
func seedTag(repos *apptest.Repos, id uint64, title string, sort int, desc string, enabled bool) *taxonomy.Tag {
	tag := taxonomy.RestoreTag(taxonomy.RestoreTagParams{
		ID:          domain.TagID(ctype.NewID(id)),
		Title:       title,
		Sort:        sort,
		Description: desc,
		Enabled:     enabled,
		CreatedBy:   admin,
	})
	repos.FakeTags().Seed(tag)
	return tag
}

func TestDeleteTagsRejectsTagStillReferencedByArticles(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	tag := seedTag(repos, 5, "在用标签", 0, "", true)
	repos.FakeTags().SeedArticleTags(ctype.NewID(1), tag.ID())
	repos.FakeTags().SeedArticleTags(ctype.NewID(2), tag.ID())

	deleted, err := svc.DeleteTags(ctx, adminViewer(), []domain.TagID{tag.ID()})
	if err == nil {
		t.Fatal("被文章引用的标签不应允许删除，实际删除成功")
	}
	if deleted != 0 {
		t.Fatalf("被拒的删除不应报告删除条数，实际 %d", deleted)
	}
	if !apperr.IsCode(err, apperr.CodeTagInUse) {
		t.Fatalf("应返回「标签被引用」错误码，实际 %v", err)
	}
	// 文案是给管理员看的：要有篇数（他才判断得出影响面），要指出去改用停用。
	for _, want := range []string{"2 篇文章在用", "停用"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("提示文案应包含 %q，实际 %q", want, err.Error())
		}
	}
	// 拒绝 = 什么都没发生：标签还在，仓储的 Delete 一次都没被调用。
	if _, findErr := repos.FakeTags().Find(ctx, tag.ID()); findErr != nil {
		t.Fatalf("被拒的删除不能真的删掉标签: %v", findErr)
	}
	if calls := repos.FakeTags().Calls("Delete"); calls != 0 {
		t.Fatalf("被拒的删除不应调用仓储 Delete，实际 %d 次", calls)
	}
	if tx.Commits() != 0 || tx.Rollbacks() != 1 {
		t.Fatalf("被拒的删除应回滚事务，实际 commits=%d rollbacks=%d", tx.Commits(), tx.Rollbacks())
	}
}

func TestDeleteTagsRejectsWholeBatchWhenAnyTagIsReferenced(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	clean := seedTag(repos, 6, "没人用的标签", 0, "", true)
	used := seedTag(repos, 7, "被人用的标签", 0, "", true)
	repos.FakeTags().SeedArticleTags(ctype.NewID(1), used.ID())

	// 批量删除是"一次动作"：其中一条被引用就整单拒绝，不能删掉一半再报错。
	if _, err := svc.DeleteTags(ctx, adminViewer(), []domain.TagID{clean.ID(), used.ID()}); err == nil {
		t.Fatal("批量里含被引用标签时应整单拒绝，实际成功了")
	}
	if _, findErr := repos.FakeTags().Find(ctx, clean.ID()); findErr != nil {
		t.Fatalf("整单拒绝时零引用标签也不该被删: %v", findErr)
	}
}

func TestDeleteTagsRemovesUnreferencedTag(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	tag := seedTag(repos, 8, "零引用标签", 0, "", true)

	deleted, err := svc.DeleteTags(ctx, adminViewer(), []domain.TagID{tag.ID()})
	if err != nil {
		t.Fatalf("零引用标签应能删除，实际 %v", err)
	}
	if deleted != 1 {
		t.Fatalf("应报告删除 1 条，实际 %d", deleted)
	}
	if _, findErr := repos.FakeTags().Find(ctx, tag.ID()); !errors.Is(findErr, taxonomy.ErrTagNotFound) {
		t.Fatalf("删除后应查不到该标签，实际 %v", findErr)
	}
	if tx.Commits() != 1 {
		t.Fatalf("删除应在一次事务内完成，实际提交 %d 次", tx.Commits())
	}
}

func TestSaveTagUpdateKeepsUnspecifiedFields(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	// 一个早已停用、排序与描述都不是零值的标签。
	tag := seedTag(repos, 9, "停用标签", 7, "旧描述", false)

	// 只传标题（后台表单只改了名字的情形）：其余字段必须原样保留。
	saved, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{ID: tag.ID(), Title: "停用标签改名"})
	if err != nil {
		t.Fatalf("只改标题应成功，实际 %v", err)
	}
	if saved.Title() != "停用标签改名" {
		t.Fatalf("标题应更新，实际 %q", saved.Title())
	}
	if saved.Enabled() {
		t.Fatal("没传启用状态时不该把停用标签悄悄放回全站可选")
	}
	if saved.Sort() != 7 {
		t.Fatalf("没传排序时应保留原值 7，实际 %d", saved.Sort())
	}
	if saved.Description() != "旧描述" {
		t.Fatalf("没传描述时应保留原值「旧描述」，实际 %q", saved.Description())
	}

	// 落库的那份也必须一样，不能只改返回值。
	stored, _ := repos.FakeTags().Find(ctx, tag.ID())
	if stored.Enabled() || stored.Sort() != 7 || stored.Description() != "旧描述" {
		t.Fatalf("库里的标签被改坏: enabled=%v sort=%d desc=%q",
			stored.Enabled(), stored.Sort(), stored.Description())
	}
}

func TestSaveTagUpdateAppliesSpecifiedFields(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	tag := seedTag(repos, 10, "老标签", 1, "老描述", false)

	sort := 42
	desc := "新描述"
	enabled := true
	saved, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{
		ID:          tag.ID(),
		Title:       " 新标签 ",
		Sort:        &sort,
		Description: &desc,
		Enabled:     &enabled,
	})
	if err != nil {
		t.Fatalf("完整更新应成功，实际 %v", err)
	}
	if saved.Title() != "新标签" {
		t.Fatalf("标题应去掉首尾空白，实际 %q", saved.Title())
	}
	if saved.Sort() != 42 || saved.Description() != "新描述" || !saved.Enabled() {
		t.Fatalf("显式传入的字段应生效，实际 sort=%d desc=%q enabled=%v",
			saved.Sort(), saved.Description(), saved.Enabled())
	}
}

func TestSaveTagUpdateUnknownTagIsNotFound(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	_, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{
		ID:    domain.TagID(ctype.NewID(404)),
		Title: "不存在",
	})
	if !errors.Is(err, taxonomy.ErrTagNotFound) {
		t.Fatalf("更新不存在的标签应返回 ErrTagNotFound，实际 %v", err)
	}
	if calls := repos.FakeTags().Calls("Save"); calls != 0 {
		t.Fatalf("找不到的标签不应写库，实际 Save 被调用 %d 次", calls)
	}
}

func TestSaveTagDuplicateTitleIsReadable(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	seedTag(repos, 11, "Go", 0, "", true)
	other := seedTag(repos, 12, "MySQL", 0, "", true)

	// 改名撞别人的标题：可读文案，不是数据库 1062 原文。
	_, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{ID: other.ID(), Title: "Go"})
	if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
		t.Fatalf("重名应返回 ErrTitleDuplicated，实际 %v", err)
	}
	if !apperr.IsCode(err, apperr.CodeTaxonomyTitleExists) {
		t.Fatalf("应带可读的 apperr 错误码，实际 %v", err)
	}
	if err.Error() != "名称重复" {
		t.Fatalf("面向用户的文案应为「名称重复」，实际 %q", err.Error())
	}
	// 自己改自己的名字（标题不变）不算重复。
	if _, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{ID: other.ID(), Title: "MySQL"}); err != nil {
		t.Fatalf("标题没变不应判定为重名，实际 %v", err)
	}
}

func TestSaveTagCreateIsEnabledByDefault(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	saved, err := svc.SaveTag(ctx, adminViewer(), app.TagInput{Title: "新标签"})
	if err != nil {
		t.Fatalf("新建标签失败: %v", err)
	}
	if !saved.Enabled() {
		t.Fatal("新建标签默认应启用")
	}
	if saved.Sort() != 0 || saved.Description() != "" {
		t.Fatalf("没传排序/描述时应落零值，实际 sort=%d desc=%q", saved.Sort(), saved.Description())
	}
	if _, findErr := repos.FakeTags().Find(ctx, saved.ID()); findErr != nil {
		t.Fatalf("新建的标签应落库: %v", findErr)
	}
}

func TestSaveTagRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.SaveTag(ctx, authorViewer(), app.TagInput{Title: "标签"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员应被拒绝，实际 %v", err)
	}
	if _, err := svc.DeleteTags(ctx, authorViewer(), []domain.TagID{ctype.NewID(1)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员删除应被拒绝，实际 %v", err)
	}
}
