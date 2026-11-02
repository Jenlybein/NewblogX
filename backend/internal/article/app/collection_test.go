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

// createCollection 是"管理员建一个合集"的简写。
func createCollection(t *testing.T, svc *app.Service, title string) collection.ID {
	t.Helper()
	id, err := svc.CreateCollection(context.Background(), app.CreateCollectionCmd{
		Actor:       adminViewer(),
		Title:       title,
		Description: "主题描述",
		Cover:       "",
		Displayed:   true,
	})
	if err != nil {
		t.Fatalf("创建合集失败: %v", err)
	}
	return id
}

// includeArticle 是"管理员把一篇文章收录进合集"的简写。
func includeArticle(t *testing.T, svc *app.Service, collectionID collection.ID, articleID domain.ArticleID) {
	t.Helper()
	if err := svc.IncludeArticle(context.Background(), app.IncludeArticleCmd{
		Actor:        adminViewer(),
		CollectionID: collectionID,
		ArticleID:    articleID,
	}); err != nil {
		t.Fatalf("收录文章失败: %v", err)
	}
}

// TestCreateCollectionRequiresAdmin 合集是管理员专属运营产物，作者不参与创建。
func TestCreateCollectionRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		_, err := svc.CreateCollection(context.Background(), app.CreateCollectionCmd{
			Actor:     viewer,
			Title:     "普通用户想建合集",
			Displayed: true,
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 建合集应被拒绝，实际 %v", name, err)
		}
	}
}

// TestCreateCollectionRejectsEmptyTitle 标题非空是合集唯一的不变量。
func TestCreateCollectionRejectsEmptyTitle(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	_, err := svc.CreateCollection(context.Background(), app.CreateCollectionCmd{
		Actor:     adminViewer(),
		Title:     "   ",
		Displayed: true,
	})
	if !apperr.IsCode(err, apperr.CodeCollectionTitleRequired) {
		t.Fatalf("空标题应被拒绝，实际 %v", err)
	}
}

// TestCreateCollectionAppendsToEnd 新建的合集排在末尾，列表按顺序返回。
func TestCreateCollectionAppendsToEnd(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	first := createCollection(t, svc, "Nuxt 部署实战")
	second := createCollection(t, svc, "分布式入门六讲")

	cards, total, err := svc.ListCollectionCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 2 || len(cards) != 2 {
		t.Fatalf("应有 2 个合集，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].ID != first || cards[1].ID != second {
		t.Fatalf("合集顺序不对: %s, %s", cards[0].ID, cards[1].ID)
	}

	adminCards, err := svc.ListAdminCollectionCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台合集列表失败: %v", err)
	}
	if len(adminCards) != 2 || adminCards[0].SortOrder != 1 || adminCards[1].SortOrder != 2 {
		t.Fatalf("后台顺序号应从 1 连续编号，实际 %+v", adminCards)
	}
}

// TestIncludeArticleAppendAndIdempotent 收录排在末尾；重复收录不产生第二行、不改序号。
func TestIncludeArticleAppendAndIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "Nuxt 部署实战")

	a1 := seedPublished(t, repos, 11)
	a2 := seedPublished(t, repos, 12)

	includeArticle(t, svc, cid, a1)
	includeArticle(t, svc, cid, a2)
	// 再收一次第一篇：既不能报重复，也不能把它挪到末尾。
	if err := svc.IncludeArticle(context.Background(), app.IncludeArticleCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		ArticleID:    a1,
	}); !apperr.IsCode(err, apperr.CodeCollectionArticleDuplicated) {
		t.Fatalf("重复收录应报【已在合集中】，实际 %v", err)
	}

	ids, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != a1 || ids[1] != a2 {
		t.Fatalf("收录顺序不对: %v", ids)
	}
}

// TestIncludeRejectsNonPublicArticle 只有"已发布且可见"的文章能被收录。
func TestIncludeRejectsNonPublicArticle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status domain.Status
		vis    domain.Visibility
	}{
		{"草稿", domain.StatusDraft, domain.VisibilityVisible},
		{"审核中", domain.StatusExamining, domain.VisibilityVisible},
		{"作者隐藏", domain.StatusPublished, domain.VisibilityUserHidden},
		{"管理员隐藏", domain.StatusPublished, domain.VisibilityAdminHidden},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos, _ := newService(t)
			cid := createCollection(t, svc, "主题合集")
			articleID := domain.ArticleID(ctype.NewID(21))
			repos.FakeArticles().Seed(apptest.NewArticle().
				ID(articleID).Author(author).Status(c.status).Visibility(c.vis).Build())

			err := svc.IncludeArticle(context.Background(), app.IncludeArticleCmd{
				Actor:        adminViewer(),
				CollectionID: cid,
				ArticleID:    articleID,
			})
			if !apperr.IsCode(err, apperr.CodeCollectionArticleNotIncludable) {
				t.Fatalf("%s 的文章应不可收录，实际 %v", c.name, err)
			}
			ids, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
			if err != nil {
				t.Fatalf("读收录失败: %v", err)
			}
			if len(ids) != 0 {
				t.Fatalf("被拒绝的文章不应落库: %v", ids)
			}
		})
	}
}

// TestIncludeRequiresAdmin 收录与创建同一条权限线。
func TestIncludeRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	articleID := seedPublished(t, repos, 31)

	err := svc.IncludeArticle(context.Background(), app.IncludeArticleCmd{
		Actor:        authorViewer(),
		CollectionID: cid,
		ArticleID:    articleID,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员收录应被拒绝，实际 %v", err)
	}
}

// TestRemoveArticleCompactsOrder 移除后剩余序号压实成 1..n，不留洞。
func TestRemoveArticleCompactsOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	a1 := seedPublished(t, repos, 41)
	a2 := seedPublished(t, repos, 42)
	a3 := seedPublished(t, repos, 43)
	includeArticle(t, svc, cid, a1)
	includeArticle(t, svc, cid, a2)
	includeArticle(t, svc, cid, a3)

	if err := svc.RemoveArticle(context.Background(), app.RemoveArticleCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		ArticleID:    a2,
	}); err != nil {
		t.Fatalf("移除收录失败: %v", err)
	}

	detail, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读合集详情失败: %v", err)
	}
	if len(detail.Articles) != 2 {
		t.Fatalf("移除后应剩 2 篇，实际 %d", len(detail.Articles))
	}
	if detail.Articles[0].SortOrder != 1 || detail.Articles[1].SortOrder != 2 {
		t.Fatalf("序号应压实为 1,2，实际 %d,%d",
			detail.Articles[0].SortOrder, detail.Articles[1].SortOrder)
	}
	if detail.Articles[0].ArticleID != a1 || detail.Articles[1].ArticleID != a3 {
		t.Fatalf("移除后剩余顺序不对: %v / %v", detail.Articles[0].ArticleID, detail.Articles[1].ArticleID)
	}
}

// TestReaderListHidesUndisplayedCollection 未展示的合集对读者不可见，后台仍可见。
func TestReaderListHidesUndisplayedCollection(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	cid := createCollection(t, svc, "还没发布的专题")
	if err := svc.SetCollectionDisplay(context.Background(), app.SetCollectionDisplayCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		Displayed:    false,
	}); err != nil {
		t.Fatalf("关闭展示失败: %v", err)
	}

	cards, total, err := svc.ListCollectionCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("未展示的合集不应出现在读者列表，实际 total=%d len=%d", total, len(cards))
	}

	adminCards, err := svc.ListAdminCollectionCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台合集列表失败: %v", err)
	}
	if len(adminCards) != 1 || adminCards[0].Displayed {
		t.Fatalf("后台应看到未展示的合集，实际 %+v", adminCards)
	}
}

// TestReaderDetailNotFoundForUndisplayed 读者直接打开未展示的合集按 404 处理。
//
// 管理员仍能打开（后台编辑要用），所以这里两路都测。
func TestReaderDetailNotFoundForUndisplayed(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	cid := createCollection(t, svc, "还没发布的专题")
	if err := svc.SetCollectionDisplay(context.Background(), app.SetCollectionDisplayCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		Displayed:    false,
	}); err != nil {
		t.Fatalf("关闭展示失败: %v", err)
	}

	if _, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false); !apperr.IsCode(err, apperr.CodeCollectionNotFound) {
		t.Fatalf("读者读未展示的合集应按不存在处理，实际 %v", err)
	}

	detail, err := svc.CollectionDetail(context.Background(), cid, adminViewer(), false)
	if err != nil {
		t.Fatalf("管理员应能读未展示的合集: %v", err)
	}
	if detail.Collection.ID() != cid {
		t.Fatalf("读到的合集不对: %s", detail.Collection.ID())
	}
}

// TestReaderDetailOrdersArticles 详情按管理员排的顺序返回文章。
func TestReaderDetailOrdersArticles(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "分布式入门六讲")
	a1 := seedPublished(t, repos, 51)
	a2 := seedPublished(t, repos, 52)
	includeArticle(t, svc, cid, a1)
	includeArticle(t, svc, cid, a2)

	detail, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读合集详情失败: %v", err)
	}
	if len(detail.Articles) != 2 {
		t.Fatalf("应有 2 篇，实际 %d", len(detail.Articles))
	}
	for i, item := range detail.Articles {
		if item.SortOrder != i+1 {
			t.Fatalf("第 %d 篇序号应为 %d，实际 %d", i+1, i+1, item.SortOrder)
		}
	}
	if detail.Articles[0].ArticleID != a1 || detail.Articles[1].ArticleID != a2 {
		t.Fatalf("顺序不对: %v / %v", detail.Articles[0].ArticleID, detail.Articles[1].ArticleID)
	}
}

// TestReplaceArticleOrder 全集顺序替换：集合不一致整单拒绝。
func TestReplaceArticleOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")
	a1 := seedPublished(t, repos, 61)
	a2 := seedPublished(t, repos, 62)
	includeArticle(t, svc, cid, a1)
	includeArticle(t, svc, cid, a2)

	if err := svc.ReplaceCollectionArticleOrder(context.Background(), adminViewer(), cid,
		[]collection.ArticleID{a2, a1}); err != nil {
		t.Fatalf("合法换序应成功: %v", err)
	}

	detail, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读合集详情失败: %v", err)
	}
	if detail.Articles[0].ArticleID != a2 {
		t.Fatalf("换序后第一篇应为 %s，实际 %s", a2, detail.Articles[0].ArticleID)
	}

	// 缺一条：整单拒绝，顺序不能被动过。
	err = svc.ReplaceCollectionArticleOrder(context.Background(), adminViewer(), cid,
		[]collection.ArticleID{a2})
	if !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
		t.Fatalf("缺一条应被拒绝，实际 %v", err)
	}
	after, err := repos.FakeCollections().ListItemIDs(context.Background(), cid)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(after) != 2 || after[0] != a2 || after[1] != a1 {
		t.Fatalf("被拒绝的排序不应改动数据，实际 %v", after)
	}
}


