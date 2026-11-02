package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「合集失效行」的服务层用例。
//
// 失效行保留在关系表里（后台仍看得到占位），读者侧由仓储 JOIN 过滤；
// 一键清理只删**失效**行，且重复调用是幂等的（第二次返回 0）。

// cleanupStale 是"管理员一键清理合集失效行"的简写。
func cleanupStale(t *testing.T, svc *app.Service, collectionID collection.ID) int {
	t.Helper()
	removed, err := svc.CleanupStaleCollectionArticles(
		context.Background(), adminViewer(), collectionID)
	if err != nil {
		t.Fatalf("清理失效行失败: %v", err)
	}
	return removed
}

// TestCollectionAdminItemsKeepStalePlaceholders 后台保留占位并标出失效原因。
//
// 三种成因（退回草稿 / 作者隐藏 / 删除）都要能被看见；一键生效的文章照常展示。
func TestCollectionAdminItemsKeepStalePlaceholders(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")

	live := seedPublished(t, repos, 91)
	draft := seedPublished(t, repos, 92)
	hidden := seedPublished(t, repos, 93)
	deleted := seedPublished(t, repos, 94)
	for _, id := range []domain.ArticleID{live, draft, hidden, deleted} {
		includeArticle(t, svc, cid, id)
	}

	// 收录之后文章状态变化：失效行必须保留在关系表里，只是读者侧看不到。
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(draft).Author(author).Title("退回草稿的文章").
			Status(domain.StatusDraft).Visibility(domain.VisibilityVisible).Build())
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(hidden).Author(author).Title("作者隐藏的文章").
			Status(domain.StatusPublished).Visibility(domain.VisibilityUserHidden).Build())
	if err := repos.FakeArticles().SoftDelete(context.Background(), deleted); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	items, err := svc.ListCollectionAdminItems(context.Background(), adminViewer(), cid)
	if err != nil {
		t.Fatalf("读后台收录行失败: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("后台应保留全部 4 个占位（含失效），实际 %d", len(items))
	}
	if got, want := []domain.ArticleID{items[0].ArticleID, items[1].ArticleID, items[2].ArticleID, items[3].ArticleID},
		[]domain.ArticleID{live, draft, hidden, deleted}; !equalArticleIDs(got, want) {
		t.Fatalf("后台行顺序不对: %v，期望 %v", got, want)
	}

	if items[0].Stale() {
		t.Fatal("仍公开的文章不应标为失效")
	}
	for i, wantReason := range []domain.StaleReason{
		domain.StaleReasonNone,
		domain.StaleReasonNotPublished,
		domain.StaleReasonNotVisible,
		domain.StaleReasonDeleted,
	} {
		if got := items[i].StaleReason(); got != wantReason {
			t.Fatalf("第 %d 行失效原因 = %q，期望 %q", i+1, got, wantReason)
		}
	}
	for i, item := range items[1:] {
		if !item.Stale() {
			t.Fatalf("第 %d 个失效占位没有标记: %+v", i+2, item)
		}
		if item.Title == "" {
			t.Fatalf("第 %d 个失效占位丢失了标题（后台无法看出是哪篇）: %+v", i+2, item)
		}
	}
}

// TestCollectionReaderHidesStaleAndCountsOnlyPublic 读者侧与合集卡片计数都不含失效行。
func TestCollectionReaderHidesStaleAndCountsOnlyPublic(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")

	live := seedPublished(t, repos, 95)
	draft := seedPublished(t, repos, 96)
	hidden := seedPublished(t, repos, 97)
	deleted := seedPublished(t, repos, 98)
	for _, id := range []domain.ArticleID{live, draft, hidden, deleted} {
		includeArticle(t, svc, cid, id)
	}

	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(draft).Author(author).Status(domain.StatusDraft).
			Visibility(domain.VisibilityVisible).Build())
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(hidden).Author(author).Status(domain.StatusPublished).
			Visibility(domain.VisibilityAdminHidden).Build())
	if err := repos.FakeArticles().SoftDelete(context.Background(), deleted); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	detail, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读读者详情失败: %v", err)
	}
	if len(detail.Articles) != 1 || detail.Articles[0].ArticleID != live {
		t.Fatalf("读者详情只应剩仍公开的 1 篇，实际 %+v", detail.Articles)
	}

	cards, total, err := svc.ListCollectionCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读读者列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 {
		t.Fatalf("应有 1 个展示中的合集，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].ArticleCount != 1 {
		t.Fatalf("卡片收录数只算仍公开的文章，期望 1，实际 %d", cards[0].ArticleCount)
	}
}

// TestCleanupStaleRemovesOnlyStaleAndIsIdempotent 一键清理只删失效行，可重复调用。
func TestCleanupStaleRemovesOnlyStaleAndIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")

	live := seedPublished(t, repos, 101)
	draft := seedPublished(t, repos, 102)
	hidden := seedPublished(t, repos, 103)
	deleted := seedPublished(t, repos, 104)
	for _, id := range []domain.ArticleID{live, draft, hidden, deleted} {
		includeArticle(t, svc, cid, id)
	}

	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(draft).Author(author).Status(domain.StatusDraft).
			Visibility(domain.VisibilityVisible).Build())
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(hidden).Author(author).Status(domain.StatusPublished).
			Visibility(domain.VisibilityUserHidden).Build())
	if err := repos.FakeArticles().SoftDelete(context.Background(), deleted); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	if removed := cleanupStale(t, svc, cid); removed != 3 {
		t.Fatalf("应清理 3 条失效行，实际 %d", removed)
	}

	ids, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != live {
		t.Fatalf("清理后应只剩仍公开的那篇 %s，实际 %v", live, ids)
	}

	items, err := svc.ListCollectionAdminItems(context.Background(), adminViewer(), cid)
	if err != nil {
		t.Fatalf("清理后读后台收录行失败: %v", err)
	}
	if len(items) != 1 || items[0].Stale() || items[0].SortOrder != 1 {
		t.Fatalf("清理后后台应只剩 1 行在架文章且序号压实为 1，实际 %+v", items)
	}

	// 幂等：再点一次不会删到生效行，返回 0。
	if removed := cleanupStale(t, svc, cid); removed != 0 {
		t.Fatalf("第二次清理应为 0，实际 %d", removed)
	}
	if ids, _ := repos.FakeCollections().ListItemIDs(context.Background(), cid); len(ids) != 1 || ids[0] != live {
		t.Fatalf("重复清理不应改动在架文章，实际 %v", ids)
	}
}

// TestCollectionStaleAdminActionsRequireAdmin 后台占位读取与一键清理都只给管理员。
func TestCollectionStaleAdminActionsRequireAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	a := seedPublished(t, repos, 111)
	includeArticle(t, svc, cid, a)

	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		if _, err := svc.ListCollectionAdminItems(context.Background(), viewer, cid); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 读后台收录行应被拒绝，实际 %v", name, err)
		}
		if _, err := svc.CleanupStaleCollectionArticles(context.Background(), viewer, cid); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 清理失效行应被拒绝，实际 %v", name, err)
		}
	}

	// 不存在的合集：清理按不存在处理，不能静默成功。
	if _, err := svc.CleanupStaleCollectionArticles(context.Background(), adminViewer(),
		collection.ID(ctype.NewID(9999))); !errors.Is(err, collection.ErrNotFound) {
		t.Fatalf("不存在的合集清理应返回不存在，实际 %v", err)
	}
}

// equalArticleIDs 比较两串文章 ID。
func equalArticleIDs(a, b []domain.ArticleID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
