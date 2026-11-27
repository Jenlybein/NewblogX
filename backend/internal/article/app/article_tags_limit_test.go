package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 一篇文章最多选 5 个标签（产品决定，domain.MaxArticleTags）。
//
// 上限由 app 层的 loadUsableTags 统一校验 —— 创建与编辑都过那里，所以这里
// 两条路径各钉一次：作者在编辑器里多勾一个，不管走"存草稿"还是"编辑重投"
// 都会拿到同一个中文错误码，而不是到写库时才炸、或者其中一个入口漏掉。

// seedUsableTags 预置 n 个可用标签，返回它们的 ID。
func seedUsableTags(t *testing.T, repos *apptest.Repos, n int) []domain.TagID {
	t.Helper()
	ids := make([]domain.TagID, 0, n)
	for i := 1; i <= n; i++ {
		id := ctype.NewID(uint64(9000 + i))
		repos.FakeTags().Seed(apptest.NewTagForTest(id, "标签", true))
		ids = append(ids, id)
	}
	return ids
}

func TestCreateRejectsMoreTagsThanLimit(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	ctx := context.Background()
	ids := seedUsableTags(t, repos, domain.MaxArticleTags+1)

	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "标签超限",
		Content:      "正文",
		TagIDs:       ids,
		TargetStatus: domain.StatusDraft,
	})
	if !apperr.IsCode(err, apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("第 %d 个标签应返回 tag_limit_exceeded，实际 %v", domain.MaxArticleTags+1, err)
	}
	// 文案必须是可读的中文，不能让作者看到一串码。
	if msg := apperr.Message(apperr.CodeArticleTagLimitExceeded); msg == "" ||
		msg == string(apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("错误码没有中文文案: %q", msg)
	}
	if tx.Rollbacks() == 0 {
		t.Fatal("标签超限应回滚事务")
	}
	if got := repos.FakeArticles().IDs(); len(got) != 0 {
		t.Fatalf("回滚后不应留下文章，实际 %v", got)
	}
}

func TestCreateAcceptsTagsUpToLimit(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	ids := seedUsableTags(t, repos, domain.MaxArticleTags)

	res, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "正好五个标签",
		Content:      "正文",
		TagIDs:       ids,
		TargetStatus: domain.StatusDraft,
	})
	if err != nil {
		t.Fatalf("正好 %d 个标签应通过，实际 %v", domain.MaxArticleTags, err)
	}
	if got := res.Article.Tags(); len(got) != domain.MaxArticleTags {
		t.Fatalf("应写入 %d 个标签，实际 %d（%v）", domain.MaxArticleTags, len(got), got)
	}
}

// 5 个标签里有一个重复：语义上仍是 5 个，不该被当成 6 个拒绝。
func TestCreateCountsTagsAfterDedupe(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	ids := seedUsableTags(t, repos, domain.MaxArticleTags)
	// 再传一次第一个：请求里是 6 个元素，但只有 5 个不同标签。
	withDuplicate := append(append([]domain.TagID{}, ids...), ids[0])

	res, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "带重复的五个标签",
		Content:      "正文",
		TagIDs:       withDuplicate,
		TargetStatus: domain.StatusDraft,
	})
	if err != nil {
		t.Fatalf("重复标签应先去重、5 个仍应通过，实际 %v", err)
	}
	if got := res.Article.Tags(); len(got) != domain.MaxArticleTags {
		t.Fatalf("去重后应写入 %d 个标签，实际 %d（%v）", domain.MaxArticleTags, len(got), got)
	}
}

// 编辑路径同样受限：不能靠"先建 5 个、再编辑加到 6 个"绕过上限。
func TestUpdateRejectsMoreTagsThanLimit(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	articleID := seedArticle(t, repos, ctype.NewID(9101), author, domain.StatusDraft).ID()
	ids := seedUsableTags(t, repos, domain.MaxArticleTags+1)

	title := "改过的标题"
	over := ids
	_, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: articleID,
		Title:     &title,
		TagIDs:    &over,
		Policy:    domain.OpenWritePolicy(),
	})
	if !apperr.IsCode(err, apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("编辑时超过上限应返回 tag_limit_exceeded，实际 %v", err)
	}
	// 失败后库里仍是原来的 0 个标签，不能写进去一半。
	stored, ferr := repos.Articles().Find(ctx, articleID)
	if ferr != nil {
		t.Fatalf("读取文章失败: %v", ferr)
	}
	if len(stored.Tags()) != 0 {
		t.Fatalf("超限失败不该改动标签，实际 %v", stored.Tags())
	}
}
