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
	"myblogx/internal/article/domain/outline"
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

// TestReaderDetailOrdersArticles 详情按目录树的 DFS 顺序返回公开文章节点。
func TestReaderDetailOrdersArticles(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "分布式入门六讲")
	a1 := seedPublished(t, repos, 51)
	a2 := seedPublished(t, repos, 52)
	seedReaderArticleNodes(t, repos, outline.OwnerCollection, outline.ID(cid), ctype.ID(a1), ctype.ID(a2))

	detail, err := svc.CollectionDetail(context.Background(), cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读合集详情失败: %v", err)
	}
	if got, want := readerDetailArticleIDs(detail.Tree), []ctype.ID{ctype.ID(a1), ctype.ID(a2)}; !sameStrings(
		func() []string {
			out := make([]string, 0, len(got))
			for _, id := range got {
				out = append(out, id.String())
			}
			return out
		}(),
		func() []string {
			out := make([]string, 0, len(want))
			for _, id := range want {
				out = append(out, id.String())
			}
			return out
		}(),
	) {
		t.Fatalf("顺序不对: %v / %v", got, want)
	}
	if orders := readerDetailSortOrders(detail.Tree); len(orders) != 2 || orders[0] != 1 || orders[1] != 2 {
		t.Fatalf("兄弟序号应为 1,2，实际 %v", orders)
	}
}

// seedPublishedWithCover 预置一篇带封面的已发布可见文章。
func seedPublishedWithCover(t *testing.T, repos *apptest.Repos, id uint64, cover string) domain.ArticleID {
	t.Helper()
	articleID := domain.ArticleID(ctype.NewID(id))
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(articleID).Author(author).Status(domain.StatusPublished).Cover(cover).Build(),
	)
	return articleID
}

// TestUpdateCollectionChangesFields 改合集只改标题 / 描述 / 封面，展示开关与顺序不动。
func TestUpdateCollectionChangesFields(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "旧标题")
	before, err := repos.FakeCollections().Find(context.Background(), cid)
	if err != nil {
		t.Fatalf("读合集失败: %v", err)
	}

	err = svc.UpdateCollection(context.Background(), app.UpdateCollectionCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		Title:        "  新标题  ",
		Description:  "新描述",
		Cover:        "https://cdn.example.com/cover.png",
	})
	if err != nil {
		t.Fatalf("改合集失败: %v", err)
	}

	after, err := repos.FakeCollections().Find(context.Background(), cid)
	if err != nil {
		t.Fatalf("读合集失败: %v", err)
	}
	if after.Title() != "新标题" {
		t.Fatalf("标题应被去空白后保存，实际 %q", after.Title())
	}
	if after.Description() != "新描述" || after.Cover() != "https://cdn.example.com/cover.png" {
		t.Fatalf("描述 / 封面未保存，实际 %q / %q", after.Description(), after.Cover())
	}
	// 改内容不是改运营位：展示开关与顺序必须原样保留。
	if after.Displayed() != before.Displayed() || after.SortOrder() != before.SortOrder() {
		t.Fatalf("改合集不应动展示开关与顺序，实际 displayed=%v order=%d", after.Displayed(), after.SortOrder())
	}
}

// TestUpdateCollectionRequiresAdminAndValidTitle 非管理员不能改；标题沿用创建时的不变量。
func TestUpdateCollectionRequiresAdminAndValidTitle(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	cid := createCollection(t, svc, "主题合集")

	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		err := svc.UpdateCollection(context.Background(), app.UpdateCollectionCmd{
			Actor:        viewer,
			CollectionID: cid,
			Title:        "偷改标题",
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 改合集应被拒绝，实际 %v", name, err)
		}
	}

	err := svc.UpdateCollection(context.Background(), app.UpdateCollectionCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
		Title:        "   ",
	})
	if !apperr.IsCode(err, apperr.CodeCollectionTitleRequired) {
		t.Fatalf("空标题应被拒绝，实际 %v", err)
	}
}

// TestDeleteCollectionKeepsArticles 删合集是软删：读者与后台列表都不再出现，
// 但合集里的文章本身不能被删掉。
func TestDeleteCollectionKeepsArticles(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "要下线的专题")
	a1 := seedPublished(t, repos, 71)
	seedReaderArticleNodes(t, repos, outline.OwnerCollection, outline.ID(cid), ctype.ID(a1))

	if err := svc.DeleteCollection(context.Background(), app.DeleteCollectionCmd{
		Actor:        authorViewer(),
		CollectionID: cid,
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员删合集应被拒绝，实际 %v", err)
	}

	if err := svc.DeleteCollection(context.Background(), app.DeleteCollectionCmd{
		Actor:        adminViewer(),
		CollectionID: cid,
	}); err != nil {
		t.Fatalf("删合集失败: %v", err)
	}

	cards, total, err := svc.ListCollectionCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读读者列表失败: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("删掉的合集不该出现在读者列表，实际 total=%d len=%d", total, len(cards))
	}

	adminCards, err := svc.ListAdminCollectionCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(adminCards) != 0 {
		t.Fatalf("删掉的合集不该出现在后台列表，实际 %+v", adminCards)
	}

	// 删合集不删文章：文章本身必须还能被读到。
	if _, err := repos.FakeArticles().Find(context.Background(), a1); err != nil {
		t.Fatalf("删合集不应删掉里面的文章: %v", err)
	}
}

// TestAdminCardsExposeFirstArticleCoverFallback 后台卡片要能算出"没有封面时读者看到的那张图"，
// 否则运营在列表里看到的是空白，而读者看到的是首篇封面。
//
// 卡片读面只看目录树（旧扁平收录表已冻结），所以这里直接播种树节点。
func TestAdminCardsExposeFirstArticleCoverFallback(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	cid := createCollection(t, svc, "无封面合集")
	first := seedPublishedWithCover(t, repos, 81, "https://cdn.example.com/first.png")
	second := seedPublishedWithCover(t, repos, 82, "https://cdn.example.com/second.png")
	seedReaderArticleNodes(t, repos, outline.OwnerCollection, outline.ID(cid), ctype.ID(first), ctype.ID(second))

	cards, err := svc.ListAdminCollectionCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("应有 1 个合集，实际 %d", len(cards))
	}
	if cards[0].Cover != "" {
		t.Fatalf("没上传过封面时 cover 应保持空串（后台据此判断该显示'回退'标记），实际 %q", cards[0].Cover)
	}
	if cards[0].FallbackCover != "https://cdn.example.com/first.png" {
		t.Fatalf("回退封面应取 DFS 第一个公开文章节点的封面，实际 %q", cards[0].FallbackCover)
	}
	if cards[0].ArticleCount != 2 {
		t.Fatalf("后台卡片篇数应只数树里仍公开的文章（2），实际 %d", cards[0].ArticleCount)
	}
}
