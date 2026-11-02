package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel/ctype"
)

// 本文件是合集编排的用例：批量收录（结果逐篇分类）、后台收录行读面、
// 合集之间的全量换序。单篇收录与合集内排序的既有用例留在 collection_test.go。

// hiddenArticle 预置一篇"已发布但被作者隐藏"的文章（不可收录、收录后也算失效）。
func hiddenArticle(t *testing.T, repos *apptest.Repos, id uint64) domain.ArticleID {
	t.Helper()
	articleID := domain.ArticleID(ctype.NewID(id))
	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(articleID).Author(author).Status(domain.StatusPublished).
		Visibility(domain.VisibilityUserHidden).Build())
	return articleID
}

// TestIncludeArticlesBatchReportsPerArticleOutcome 批量收录一次提交多篇，
// 结果逐篇分类：成功的进末尾，已收录的报重复，不可收录的报原因。
func TestIncludeArticlesBatchReportsPerArticleOutcome(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "分布式入门六讲")

	first := seedPublished(t, repos, 101)
	second := seedPublished(t, repos, 102)
	third := seedPublished(t, repos, 103)
	hidden := hiddenArticle(t, repos, 104)

	// 先单篇收一篇，它会在批量请求里被认成"重复"。
	includeArticle(t, svc, cid, first)

	results, err := svc.IncludeArticles(context.Background(), app.IncludeArticlesCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		ArticleIDs:   []domain.ArticleID{second, first, hidden, third, first},
	})
	if err != nil {
		t.Fatalf("批量收录失败: %v", err)
	}

	want := []struct {
		id      domain.ArticleID
		outcome collection.IncludeOutcome
	}{
		{second, collection.IncludeOutcomeIncluded},
		{first, collection.IncludeOutcomeDuplicated},
		{hidden, collection.IncludeOutcomeNotIncludable},
		{third, collection.IncludeOutcomeIncluded},
		{first, collection.IncludeOutcomeDuplicated},
	}
	if len(results) != len(want) {
		t.Fatalf("结果应逐篇返回（%d 条），实际 %d 条: %+v", len(want), len(results), results)
	}
	for i, w := range want {
		if results[i].ArticleID != w.id || results[i].Outcome != w.outcome {
			t.Fatalf("第 %d 条结果应为 (%s, %s)，实际 (%s, %s)",
				i, w.id, w.outcome, results[i].ArticleID, results[i].Outcome)
		}
	}

	// 只有真正新增的两篇落库，且按提交顺序排在已有那篇之后。
	ids, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 3 || ids[0] != first || ids[1] != second || ids[2] != third {
		t.Fatalf("批量收录后的顺序不对: %v", ids)
	}
}

// TestIncludeArticlesBatchReportsMissingArticle 不存在的文章不该让整单失败，
// 而是逐篇返回"不存在"，其余照常收录。
func TestIncludeArticlesBatchReportsMissingArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	present := seedPublished(t, repos, 111)
	missing := domain.ArticleID(ctype.NewID(999))

	results, err := svc.IncludeArticles(context.Background(), app.IncludeArticlesCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		ArticleIDs:   []domain.ArticleID{missing, present},
	})
	if err != nil {
		t.Fatalf("批量收录失败: %v", err)
	}
	if results[0].Outcome != collection.IncludeOutcomeMissing {
		t.Fatalf("不存在的文章应报 missing，实际 %s", results[0].Outcome)
	}
	if results[1].Outcome != collection.IncludeOutcomeIncluded {
		t.Fatalf("存在的文章应照常收录，实际 %s", results[1].Outcome)
	}
}

// TestIncludeArticlesBatchRequiresAdmin 批量收录与单篇同一条权限线，
// 被拒绝时一篇都不能落库。
func TestIncludeArticlesBatchRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	a1 := seedPublished(t, repos, 121)
	a2 := seedPublished(t, repos, 122)

	_, err := svc.IncludeArticles(context.Background(), app.IncludeArticlesCmd{
		Actor:        authorViewer(),
		CollectionID: cid,
		ArticleIDs:   []domain.ArticleID{a1, a2},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员批量收录应被拒绝，实际 %v", err)
	}
	ids, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("被拒绝的批量收录不应落库: %v", ids)
	}
}

// TestListCollectionAdminItemsIncludesOrderAndStale 后台收录行读面：
// 按顺序返回、带上标题、并标出已失效的收录（排序要拿完整集合去提交，所以失效行也不能漏）。
func TestListCollectionAdminItemsIncludesOrderAndStale(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	a1 := seedPublished(t, repos, 131)
	a2 := seedPublished(t, repos, 132)
	includeArticle(t, svc, cid, a1)
	includeArticle(t, svc, cid, a2)

	// 收录后文章变私有：收录行保留（后台要能看见并清理），读者侧由仓储过滤。
	hiddenArticle(t, repos, 132)

	items, err := svc.ListCollectionAdminItems(context.Background(), adminViewer(), cid)
	if err != nil {
		t.Fatalf("读后台收录行失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应收录 2 行，实际 %d", len(items))
	}
	if items[0].ArticleID != a1 || items[1].ArticleID != a2 {
		t.Fatalf("收录行顺序不对: %v / %v", items[0].ArticleID, items[1].ArticleID)
	}
	if items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应连续: %d / %d", items[0].SortOrder, items[1].SortOrder)
	}
	if items[0].Stale() || !items[1].Stale() {
		t.Fatalf("失效标记不对: %v / %v", items[0].Stale(), items[1].Stale())
	}

	if _, err := svc.ListCollectionAdminItems(context.Background(), authorViewer(), cid); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员读后台收录行应被拒绝，实际 %v", err)
	}
}

// TestReplaceCollectionSortOrderReordersReaderList 合集之间的手工排序：
// 后台读面与读者列表都按新的 sort_order 返回。
func TestReplaceCollectionSortOrderReordersReaderList(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	first := createCollection(t, svc, "第一个")
	second := createCollection(t, svc, "第二个")
	third := createCollection(t, svc, "第三个")

	if err := svc.ReplaceCollectionSortOrder(context.Background(), adminViewer(),
		[]collection.ID{third, first, second}); err != nil {
		t.Fatalf("合集换序失败: %v", err)
	}

	cards, total, err := svc.ListCollectionCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读读者列表失败: %v", err)
	}
	if total != 3 || len(cards) != 3 {
		t.Fatalf("应有 3 个合集，实际 total=%d len=%d", total, len(cards))
	}
	got := []collection.ID{cards[0].ID, cards[1].ID, cards[2].ID}
	if got[0] != third || got[1] != first || got[2] != second {
		t.Fatalf("读者列表顺序不对: %v", got)
	}

	adminCards, err := svc.ListAdminCollectionCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	for i, id := range []collection.ID{third, first, second} {
		if adminCards[i].ID != id || adminCards[i].SortOrder != i+1 {
			t.Fatalf("后台第 %d 行应为 (%s, %d)，实际 (%s, %d)",
				i+1, id, i+1, adminCards[i].ID, adminCards[i].SortOrder)
		}
	}
}

// TestReplaceCollectionSortOrderRejectsMismatch 全集替换：集合不一致整单拒绝，
// 顺序不能被动过。否则一个过期页面点一次「上移」就会把别人刚建的合集排丢。
func TestReplaceCollectionSortOrderRejectsMismatch(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := createCollection(t, svc, "第一个")
	second := createCollection(t, svc, "第二个")

	// 缺一个合集：整单拒绝。
	err := svc.ReplaceCollectionSortOrder(context.Background(), adminViewer(), []collection.ID{second})
	if !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
		t.Fatalf("集合不一致应报顺序冲突，实际 %v", err)
	}
	ids, err := repos.FakeCollections().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("读合集顺序失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != first || ids[1] != second {
		t.Fatalf("被拒绝的换序不应改动数据，实际 %v", ids)
	}

	// 混入不存在的合集：同样整单拒绝。
	err = svc.ReplaceCollectionSortOrder(context.Background(), adminViewer(),
		[]collection.ID{first, collection.ID(ctype.NewID(777))})
	if !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
		t.Fatalf("混入未知合集应被拒绝，实际 %v", err)
	}

	// 非管理员不能排合集之间的顺序。
	err = svc.ReplaceCollectionSortOrder(context.Background(), authorViewer(), []collection.ID{second, first})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员换序应被拒绝，实际 %v", err)
	}
}
