package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 站点「发布与展示」开关的判定（#64，ADR-0017 决策 2）。
// 与 service_test.go 拆开是为了守住 R6：单文件不得超过 800 行。

// ── 站点「发布与展示」开关（#64）──

// TestCreateDeniedBySitePolicy 验证两个开关对新建路径的拦截：
// 关草稿 → 不能新建草稿；关发布 → 不能"创建即提交审核/发布"。
// 被拒时一个字节都不该落库（判定在事务之前）。
func TestCreateDeniedBySitePolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		policy domain.WritePolicy
		target domain.Status
		want   error
	}{
		{
			"draft_off/create_draft",
			domain.WritePolicy{AllowNonAdminPublish: true, AllowNonAdminDraft: false},
			domain.StatusDraft,
			domain.ErrDraftDisabledBySite,
		},
		{
			"publish_off/create_submit",
			domain.WritePolicy{AllowNonAdminPublish: false, AllowNonAdminDraft: true},
			domain.StatusExamining,
			domain.ErrPublishDisabledBySite,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos, tx := newService(t)

			_, err := svc.Create(context.Background(), app.CreateArticleCmd{
				Actor:        authorViewer(),
				Policy:       c.policy,
				Title:        "被站点策略拦下的文章",
				Content:      "正文",
				TargetStatus: c.target,
			})
			if !errors.Is(err, c.want) {
				t.Fatalf("期望 %v，实际 %v", c.want, err)
			}
			if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
				t.Fatalf("被拒时不应写入文章，实际 %v", ids)
			}
			if tx.Commits() != 0 {
				t.Fatalf("被拒时不应提交事务，实际 %d 次", tx.Commits())
			}
		})
	}
}

// TestUpdateDeniedBySitePolicy 验证两个开关对更新路径的拦截。
//
// 覆盖 #64 验收标准的三句话：
//   - 关闭发布：更新已发布文章、发布自己的草稿、编辑审核中的文章都被拒；
//   - 关闭草稿：保存草稿、退回草稿被拒；
//   - 两个都关：草稿也保存不了。
//
// 被拒后内存里的聚合也不得被改写（判定必须在 ApplyEdit 之前）。
func TestUpdateDeniedBySitePolicy(t *testing.T) {
	t.Parallel()

	draft := domain.StatusDraft
	examining := domain.StatusExamining
	publishOff := domain.WritePolicy{AllowNonAdminPublish: false, AllowNonAdminDraft: true}
	draftOff := domain.WritePolicy{AllowNonAdminPublish: true, AllowNonAdminDraft: false}
	bothOff := domain.WritePolicy{}

	cases := []struct {
		name   string
		status domain.Status
		policy domain.WritePolicy
		target *domain.Status
		want   error
	}{
		{"draft_off/save_draft", domain.StatusDraft, draftOff, nil, domain.ErrDraftDisabledBySite},
		{"draft_off/back_to_draft", domain.StatusExamining, draftOff, &draft, domain.ErrDraftDisabledBySite},
		{"publish_off/edit_published", domain.StatusPublished, publishOff, nil, domain.ErrPublishDisabledBySite},
		{"publish_off/edit_examining", domain.StatusExamining, publishOff, nil, domain.ErrPublishDisabledBySite},
		{"publish_off/publish_draft", domain.StatusDraft, publishOff, &examining, domain.ErrPublishDisabledBySite},
		{"both_off/save_draft", domain.StatusDraft, bothOff, nil, domain.ErrDraftDisabledBySite},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos, _ := newService(t)
			seeded := seedArticle(t, repos, ctype.NewID(1), author, c.status)
			originalContent := seeded.Content()

			content := "改过的正文"
			_, err := svc.Update(context.Background(), app.UpdateArticleCmd{
				Actor:        authorViewer(),
				Policy:       c.policy,
				ArticleID:    ctype.NewID(1),
				Content:      &content,
				TargetStatus: c.target,
			})
			if !errors.Is(err, c.want) {
				t.Fatalf("期望 %v，实际 %v", c.want, err)
			}
			if got := repos.FakeArticles().Calls("Update"); got != 0 {
				t.Fatalf("被拒时不应写库，实际调用 Update %d 次", got)
			}

			found, findErr := repos.FakeArticles().Find(context.Background(), ctype.NewID(1))
			if findErr != nil {
				t.Fatalf("文章应仍然可读: %v", findErr)
			}
			if found.Content() != originalContent {
				t.Fatalf("被拒时正文不应被改写，实际 %q", found.Content())
			}
			if found.Status() != c.status {
				t.Fatalf("被拒时状态不应变化，期望 %s，实际 %s", c.status, found.Status())
			}
		})
	}
}

// TestAdminIgnoresSitePolicy 验证管理员不受两个开关影响。
func TestAdminIgnoresSitePolicy(t *testing.T) {
	t.Parallel()

	bothOff := domain.WritePolicy{}
	ctx := context.Background()

	svc, repos, _ := newService(t)

	if _, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        adminViewer(),
		Policy:       bothOff,
		Title:        "管理员的草稿",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
	}); err != nil {
		t.Fatalf("管理员应能新建草稿: %v", err)
	}

	draft := seedArticle(t, repos, ctype.NewID(2), admin, domain.StatusDraft)
	content := "管理员的修改"
	if _, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     adminViewer(),
		Policy:    bothOff,
		ArticleID: draft.ID(),
		Content:   &content,
	}); err != nil {
		t.Fatalf("管理员应能更新草稿: %v", err)
	}
}

// TestExistingDraftStillReadableAndDeletable 是"存量草稿不是孤儿"的兜底语义：
// 两个开关都关时，作者仍能打开与删除自己的草稿（只是不能改、不能发布）。
func TestExistingDraftStillReadableAndDeletable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, repos, _ := newService(t)
	seedArticle(t, repos, ctype.NewID(1), author, domain.StatusDraft)

	if _, err := svc.Detail(ctx, ctype.NewID(1), authorViewer(), false); err != nil {
		t.Fatalf("存量草稿应仍可打开: %v", err)
	}
	if _, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: ctype.NewID(1)}); err != nil {
		t.Fatalf("存量草稿应仍可删除: %v", err)
	}
}
