package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/hottop"
	"myblogx/internal/kernel/ctype"
)

// seedPublished 预置一篇已发布且可见的文章，并返回它的 ID。
func seedPublished(t *testing.T, repos *apptest.Repos, id uint64) domain.ArticleID {
	t.Helper()
	articleID := domain.ArticleID(ctype.NewID(id))
	seedArticle(t, repos, articleID, author, domain.StatusPublished)
	return articleID
}

// pin 是"管理员把这篇文章设为热门置顶"的简写。
func pin(t *testing.T, svc *app.Service, repos *apptest.Repos, id uint64) domain.ArticleID {
	t.Helper()
	articleID := seedPublished(t, repos, id)
	if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: articleID,
	}); err != nil {
		t.Fatalf("设置热门置顶失败: %v", err)
	}
	return articleID
}

// TestSetHotTopAppendsToEnd 新置顶排末尾：序号 = 当前条数 + 1。
func TestSetHotTopAppendsToEnd(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	pin(t, svc, repos, 1)
	pin(t, svc, repos, 2)
	pin(t, svc, repos, 3)

	items, err := repos.FakeHotTops().ListAdminItems(context.Background())
	if err != nil {
		t.Fatalf("读置顶失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("应有 3 条置顶，实际 %d", len(items))
	}
	for i, it := range items {
		if it.SortOrder != i+1 {
			t.Fatalf("第 %d 条序号应为 %d，实际 %d", i+1, i+1, it.SortOrder)
		}
	}
}

// TestSetHotTopIsIdempotent 重复设置不产生重复行、也不改序号。
func TestSetHotTopIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, tx := newService(t)
	articleID := pin(t, svc, repos, 1)
	second := pin(t, svc, repos, 2)

	// 再设一次第一条：序号不能变（不能把它顶到末尾）。
	if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: articleID,
	}); err != nil {
		t.Fatalf("重复设置应幂等成功，实际 %v", err)
	}

	ids, err := repos.FakeHotTops().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("读置顶失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != articleID || ids[1] != second {
		t.Fatalf("重复设置不应改动顺序，实际 %v", ids)
	}
	if tx.Commits() != 3 {
		// 两次设置 + 一次幂等空操作，各自一个事务（重复设置也要先读一遍才知道已存在）。
		t.Fatalf("应各一次事务共 3 次提交，实际 %d 次", tx.Commits())
	}
}

// TestSetHotTopRejectsNonAdmin 只有管理员能设置运营位。
func TestSetHotTopRejectsNonAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	articleID := seedPublished(t, repos, 1)

	// 连文章作者本人也不行：热门置顶是站点运营位，不是个人置顶。
	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		err := svc.SetHotTop(context.Background(), app.HotTopCmd{Actor: viewer, ArticleID: articleID})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 设置热门置顶应被拒绝，实际 %v", name, err)
		}
	}
}

// TestSetHotTopRejectsNonPublicArticle 只有"已发布且可见"的文章可置顶。
func TestSetHotTopRejectsNonPublicArticle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     domain.Status
		visibility domain.Visibility
	}{
		{"草稿", domain.StatusDraft, domain.VisibilityVisible},
		{"审核中", domain.StatusExamining, domain.VisibilityVisible},
		{"已拒绝", domain.StatusRejected, domain.VisibilityVisible},
		{"作者隐藏", domain.StatusPublished, domain.VisibilityUserHidden},
		{"管理员隐藏", domain.StatusPublished, domain.VisibilityAdminHidden},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos, _ := newService(t)
			articleID := domain.ArticleID(ctype.NewID(1))
			repos.FakeArticles().Seed(apptest.NewArticle().
				ID(articleID).Author(author).Status(c.status).Visibility(c.visibility).Build())

			err := svc.SetHotTop(context.Background(), app.HotTopCmd{
				Actor:     adminViewer(),
				ArticleID: articleID,
			})
			if !apperr.IsCode(err, apperr.CodeHotTopNotPinnable) {
				t.Fatalf("%s 应报不可置顶，实际 %v", c.name, err)
			}
		})
	}
}

// TestSetHotTopEnforcesLimit 第 13 条给出可读上限错误。
func TestSetHotTopEnforcesLimit(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	for i := uint64(1); i <= hottop.MaxItems; i++ {
		pin(t, svc, repos, i)
	}

	over := seedPublished(t, repos, 100)
	err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: over,
	})
	if !apperr.IsCode(err, apperr.CodeHotTopLimitExceeded) {
		t.Fatalf("超过上限应报 hot_top.limit_exceeded，实际 %v", err)
	}
	if msg := err.Error(); msg == "" {
		t.Fatal("上限错误必须有可读文案")
	}

	// 满员时"重复设置已有条目"仍然成功：幂等优先于上限。
	ids, _ := repos.FakeHotTops().ListIDs(context.Background())
	if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: ids[0],
	}); err != nil {
		t.Fatalf("满员时重复设置已存在条目应成功，实际 %v", err)
	}
}

// TestRemoveHotTopCompactsOrder 取消一条后序号压实，不留空洞。
func TestRemoveHotTopCompactsOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := pin(t, svc, repos, 1)
	pin(t, svc, repos, 2)
	third := pin(t, svc, repos, 3)

	if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: first,
		Remove:    true,
	}); err != nil {
		t.Fatalf("取消置顶失败: %v", err)
	}

	ids, err := repos.FakeHotTops().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("读置顶失败: %v", err)
	}
	if len(ids) != 2 || ids[1] != third {
		t.Fatalf("剩余顺序不对，实际 %v", ids)
	}
	items, _ := repos.FakeHotTops().ListAdminItems(context.Background())
	if items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应压实为 1,2，实际 %d,%d", items[0].SortOrder, items[1].SortOrder)
	}
}

// TestRemoveHotTopIsIdempotent 取消不存在的置顶不报错。
func TestRemoveHotTopIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	articleID := seedPublished(t, repos, 1)

	for i := 0; i < 2; i++ {
		if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
			Actor:     adminViewer(),
			ArticleID: articleID,
			Remove:    true,
		}); err != nil {
			t.Fatalf("第 %d 次取消置顶应幂等成功，实际 %v", i+1, err)
		}
	}
}

// TestRemoveHotTopWorksForDeletedArticle 文章被删掉后，失效的置顶行仍能清理。
//
// 这条容易漏：如果取消时先去查文章（为了做权限/状态判断），
// 文章不存在就会先返回"文章不存在"，后台的「一键清理」永远清不掉失效行。
func TestRemoveHotTopWorksForDeletedArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	articleID := pin(t, svc, repos, 1)
	if err := repos.FakeArticles().SoftDelete(context.Background(), articleID); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	if err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: articleID,
		Remove:    true,
	}); err != nil {
		t.Fatalf("清理失效置顶不应依赖文章存在，实际 %v", err)
	}
	ids, _ := repos.FakeHotTops().ListIDs(context.Background())
	if len(ids) != 0 {
		t.Fatalf("失效行应被清掉，实际 %v", ids)
	}
}

// TestSetHotTopUnknownArticle 文章不存在时返回文章不存在（不泄露其它信息）。
func TestSetHotTopUnknownArticle(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	err := svc.SetHotTop(context.Background(), app.HotTopCmd{
		Actor:     adminViewer(),
		ArticleID: domain.ArticleID(ctype.NewID(999)),
	})
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("应报文章不存在，实际 %v", err)
	}
}

// TestReplaceHotTopOrder 全量顺序替换：按给定顺序原子重排。
func TestReplaceHotTopOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := pin(t, svc, repos, 1)
	second := pin(t, svc, repos, 2)
	third := pin(t, svc, repos, 3)

	if err := svc.ReplaceHotTopOrder(context.Background(), adminViewer(),
		[]domain.ArticleID{third, first, second}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}

	ids, err := repos.FakeHotTops().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("读置顶失败: %v", err)
	}
	want := []domain.ArticleID{third, first, second}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("重排后顺序应为 %v，实际 %v", want, ids)
		}
	}
}

// TestReplaceHotTopOrderRejectsMismatch 集合不一致时整单拒绝，不做部分写入。
func TestReplaceHotTopOrderRejectsMismatch(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := pin(t, svc, repos, 1)
	pin(t, svc, repos, 2)

	err := svc.ReplaceHotTopOrder(context.Background(), adminViewer(), []domain.ArticleID{first})
	if !apperr.IsCode(err, apperr.CodeHotTopOrderMismatch) {
		t.Fatalf("少给一条应报顺序不一致，实际 %v", err)
	}

	ids, _ := repos.FakeHotTops().ListIDs(context.Background())
	if len(ids) != 2 {
		t.Fatalf("校验失败时不应改动任何行，实际剩 %d 条", len(ids))
	}
}

// TestReplaceHotTopOrderRollsBackOnFailure 重排中途失败时整单回滚，不留半套序号。
func TestReplaceHotTopOrderRollsBackOnFailure(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := pin(t, svc, repos, 1)
	second := pin(t, svc, repos, 2)

	repos.FakeHotTops().FailNext("UpdateSortOrder", errors.New("db down"))

	err := svc.ReplaceHotTopOrder(context.Background(), adminViewer(),
		[]domain.ArticleID{second, first})
	if err == nil {
		t.Fatal("仓储失败时应把错误传出去")
	}

	ids, listErr := repos.FakeHotTops().ListIDs(context.Background())
	if listErr != nil {
		t.Fatalf("读置顶失败: %v", listErr)
	}
	if len(ids) != 2 || ids[0] != first || ids[1] != second {
		t.Fatalf("失败后顺序不应改变，实际 %v", ids)
	}
}

// TestReplaceHotTopOrderRequiresAdmin 只有管理员能重排。
func TestReplaceHotTopOrderRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	err := svc.ReplaceHotTopOrder(context.Background(), authorViewer(), nil)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员重排应被拒绝，实际 %v", err)
	}
}

// TestListHotTopCards 公开读取直接把仓储的卡片列表透出（过滤已在仓储 SQL 里）。
//
// 开关关掉后的过滤形态见 visibility_test.go 的 TestHotTopCardsThreadReadFilter。
func TestListHotTopCards(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	repos.FakeHotTops().SeedCards(hottop.Card{
		ArticleID: domain.ArticleID(ctype.NewID(7)),
		Title:     "首页卡片",
		Cover:     "https://example.com/cover.png",
		ViewCount: 42,
	})

	cards, err := svc.ListHotTopCards(context.Background(), false)
	if err != nil {
		t.Fatalf("读热门置顶失败: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "首页卡片" || cards[0].ViewCount != 42 {
		t.Fatalf("卡片列表不符: %+v", cards)
	}
}

// TestListHotTopItemsRequiresAdmin 后台列表含失效行，因此只给管理员。
func TestListHotTopItemsRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	live := domain.ArticleID(ctype.NewID(7))
	gone := domain.ArticleID(ctype.NewID(8))
	repos.FakeHotTops().
		SeedEntry(live, &hottop.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}).
		SeedEntry(gone, nil).
		SeedCards(hottop.Card{ArticleID: live, Title: "在架卡片"})

	if _, err := svc.ListHotTopItems(context.Background(), authorViewer()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员读后台列表应被拒绝，实际 %v", err)
	}

	items, err := svc.ListHotTopItems(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("管理员读后台列表失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("后台列表应含 2 行（含失效），实际 %d", len(items))
	}
	if items[0].ArticleID != live || items[0].Title != "在架卡片" || items[0].Stale() {
		t.Fatalf("在架行不符: %+v", items[0])
	}
	if items[1].ArticleID != gone || !items[1].Stale() {
		t.Fatalf("文章已不存在的行应判为失效: %+v", items[1])
	}
}
