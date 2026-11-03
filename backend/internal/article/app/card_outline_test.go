package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「列表卡片读模型」的应用层契约：篇数与封面回退只看目录树，
// 旧扁平收录中间表已冻结、不再参与。
//
// 断言的是可观察结果（卡片篇数 / 封面、读者详情篇数），不测私有辅助函数。

// seedCardArticle 预置一篇状态 / 可见性 / 作者 / 封面都可控的文章。
func seedCardArticle(
	t *testing.T, repos *apptest.Repos, id uint64,
	authorID domain.UserID, status domain.Status, visibility domain.Visibility, cover string,
) domain.ArticleID {
	t.Helper()
	a := apptest.NewArticle().
		ID(ctype.NewID(id)).
		Author(authorID).
		Status(status).
		Visibility(visibility).
		Cover(cover).
		Build()
	repos.FakeArticles().Seed(a)
	return a.ID()
}

// seedCardArticleNode 把一个文章节点挂到树里。
func seedCardArticleNode(
	t *testing.T, repos *apptest.Repos, ownerKind outline.OwnerKind, ownerID ctype.ID,
	nodeID, parentID, articleID uint64, order int,
) {
	t.Helper()
	seedReaderOutline(t, repos, ownerKind, ownerID, readerNodeSpec{
		nodeID: ctype.NewID(nodeID), parentID: ctype.NewID(parentID), kind: outline.NodeKindArticle,
		articleID: ctype.NewID(articleID), order: order,
	})
}

// seedCardFolderNode 把一个文件夹节点挂到树里。
func seedCardFolderNode(
	t *testing.T, repos *apptest.Repos, ownerKind outline.OwnerKind, ownerID ctype.ID,
	nodeID, parentID uint64, title string, order int,
) {
	t.Helper()
	seedReaderOutline(t, repos, ownerKind, ownerID, readerNodeSpec{
		nodeID: ctype.NewID(nodeID), parentID: ctype.NewID(parentID), kind: outline.NodeKindFolder,
		title: title, order: order,
	})
}

// TestCollectionCardCountAndCoverComeFromOutlineTree 树里有公开文章、旧中间表为空时，
// 卡片篇数与封面回退都来自目录树；后台卡片沿用同一口径。
func TestCollectionCardCountAndCoverComeFromOutlineTree(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	cid := createCollection(t, svc, "目录树合集")
	ownerID := outline.ID(cid)

	first := seedCardArticle(t, repos, 11, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/first.png")
	second := seedCardArticle(t, repos, 12, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/second.png")
	draft := seedCardArticle(t, repos, 13, author, domain.StatusDraft, domain.VisibilityVisible, "")
	hidden := seedCardArticle(t, repos, 14, author, domain.StatusPublished, domain.VisibilityUserHidden, "")
	deleted := seedCardArticle(t, repos, 15, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/deleted.png")
	if err := repos.FakeArticles().SoftDelete(ctx, deleted); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 旧中间表已随旧接口一起下线（#225）：本用例只往树里播种，
	// 卡片篇数与封面回退必须全部来自树。
	seedCardFolderNode(t, repos, outline.OwnerCollection, ownerID, 1, 0, "第一章", 1)
	seedCardArticleNode(t, repos, outline.OwnerCollection, ownerID, 2, 1, first.Uint64(), 1)
	seedCardArticleNode(t, repos, outline.OwnerCollection, ownerID, 3, 1, second.Uint64(), 2)
	seedCardArticleNode(t, repos, outline.OwnerCollection, ownerID, 4, 0, draft.Uint64(), 2)
	seedCardArticleNode(t, repos, outline.OwnerCollection, ownerID, 5, 0, hidden.Uint64(), 3)
	seedCardArticleNode(t, repos, outline.OwnerCollection, ownerID, 6, 0, deleted.Uint64(), 4)

	cards, total, err := svc.ListCollectionCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 {
		t.Fatalf("应有 1 个合集卡片，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].ArticleCount != 2 {
		t.Fatalf("卡片篇数应只数树里仍公开的文章（2），实际 %d", cards[0].ArticleCount)
	}
	if cards[0].Cover != "https://cdn.example/first.png" {
		t.Fatalf("没封面时应回退到 DFS 第一个公开文章节点的封面，实际 %q", cards[0].Cover)
	}

	adminCards, err := svc.ListAdminCollectionCards(ctx, adminViewer())
	if err != nil {
		t.Fatalf("读后台合集列表失败: %v", err)
	}
	if len(adminCards) != 1 {
		t.Fatalf("后台应有 1 个合集，实际 %d", len(adminCards))
	}
	if adminCards[0].ArticleCount != 2 {
		t.Fatalf("后台卡片篇数应与读者同口径（2），实际 %d", adminCards[0].ArticleCount)
	}
	if adminCards[0].Cover != "" || adminCards[0].FallbackCover != "https://cdn.example/first.png" {
		t.Fatalf("后台 cover 应保持原值、回退封面取树里首个公开文章，实际 cover=%q fallback=%q",
			adminCards[0].Cover, adminCards[0].FallbackCover)
	}

	// 详情页与卡片共用同一份遍历：篇数一致。
	detail, err := svc.CollectionDetail(ctx, cid, domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("读合集详情失败: %v", err)
	}
	if detail.ArticleCount != 2 || detail.Cover != "https://cdn.example/first.png" {
		t.Fatalf("详情篇数/封面与卡片不一致: count=%d cover=%q", detail.ArticleCount, detail.Cover)
	}
}

// TestCollectionCardDoesNotBorrowCoverFromLaterArticle 第一个公开文章节点没有封面时，
// 卡片封面就是空；不能借用后一篇的封面。
func TestCollectionCardDoesNotBorrowCoverFromLaterArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	cid := createCollection(t, svc, "首篇无封面")

	first := seedCardArticle(t, repos, 21, author, domain.StatusPublished, domain.VisibilityVisible, "")
	second := seedCardArticle(t, repos, 22, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/second.png")
	seedCardArticleNode(t, repos, outline.OwnerCollection, outline.ID(cid), 1, 0, first.Uint64(), 1)
	seedCardArticleNode(t, repos, outline.OwnerCollection, outline.ID(cid), 2, 0, second.Uint64(), 2)

	cards, _, err := svc.ListCollectionCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if cards[0].ArticleCount != 2 {
		t.Fatalf("两篇都公开，篇数应为 2，实际 %d", cards[0].ArticleCount)
	}
	if cards[0].Cover != "" {
		t.Fatalf("首个公开文章节点没有封面时不应借用后一篇，实际 %q", cards[0].Cover)
	}
}

// TestCollectionCardHonorsShowNonAdminSwitch 「显示非管理员发布文章」关闭时，
// 非管理员作者的文章不计入篇数、不参与封面回退；一篇都不剩的合集不出现。
func TestCollectionCardHonorsShowNonAdminSwitch(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	repos.FakeUsers().Seed(app.AuthorView{ID: domain.UserID(admin), Admin: true})

	cid := createCollection(t, svc, "作者混合合集")
	normal := seedCardArticle(t, repos, 31, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/normal.png")
	adminAuthored := seedCardArticle(t, repos, 32, domain.UserID(admin), domain.StatusPublished,
		domain.VisibilityVisible, "https://cdn.example/admin.png")
	// 首节点是普通作者（开关关闭时不可见），第二篇才是管理员作者。
	seedCardArticleNode(t, repos, outline.OwnerCollection, outline.ID(cid), 1, 0, normal.Uint64(), 1)
	seedCardArticleNode(t, repos, outline.OwnerCollection, outline.ID(cid), 2, 0, adminAuthored.Uint64(), 2)

	// 开关打开：两篇都算，封面取首节点（普通作者那张）。
	cards, total, err := svc.ListCollectionCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || cards[0].ArticleCount != 2 || cards[0].Cover != "https://cdn.example/normal.png" {
		t.Fatalf("开关打开时卡片不对: total=%d cards=%+v", total, cards)
	}

	// 开关关闭：只剩管理员作者那篇，封面也要跳过不可见节点。
	cards, total, err = svc.ListCollectionCards(ctx, true, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || cards[0].ArticleCount != 1 || cards[0].Cover != "https://cdn.example/admin.png" {
		t.Fatalf("开关关闭时卡片不对: total=%d cards=%+v", total, cards)
	}

	// 后台卡片不看这个读者开关，仍按"公开文章"口径数两篇。
	adminCards, err := svc.ListAdminCollectionCards(ctx, adminViewer())
	if err != nil {
		t.Fatalf("读后台合集列表失败: %v", err)
	}
	if adminCards[0].ArticleCount != 2 || adminCards[0].FallbackCover != "https://cdn.example/normal.png" {
		t.Fatalf("后台卡片口径不对: %+v", adminCards[0])
	}

	// 只有非管理员作者文章的合集，开关关闭时读者列表整张卡片都不出现。
	onlyNormal := createCollection(t, svc, "只有普通作者")
	seedCardArticleNode(t, repos, outline.OwnerCollection, outline.ID(onlyNormal), 7, 0, normal.Uint64(), 1)
	cards, total, err = svc.ListCollectionCards(ctx, true, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != cid {
		t.Fatalf("开关关闭时无公开文章的合集应消失，实际 total=%d cards=%+v", total, cards)
	}
}

// TestProjectCardCountAndCoverComeFromOutlineTree 项目卡片与合集卡片同一口径：
// 篇数只数树里仍公开的文章，封面回退取 DFS 第一个公开文章节点。
func TestProjectCardCountAndCoverComeFromOutlineTree(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	pid := createProject(t, svc, "目录树项目")

	first := seedCardArticle(t, repos, 41, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/first.png")
	draft := seedCardArticle(t, repos, 42, author, domain.StatusDraft, domain.VisibilityVisible, "")
	seedCardArticleNode(t, repos, outline.OwnerProject, outline.ID(pid), 1, 0, first.Uint64(), 1)
	seedCardArticleNode(t, repos, outline.OwnerProject, outline.ID(pid), 2, 0, draft.Uint64(), 2)

	cards, total, err := svc.ListProjectCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ArticleCount != 1 {
		t.Fatalf("项目卡片篇数应只数公开文章（1），实际 total=%d cards=%+v", total, cards)
	}
	if cards[0].Cover != "https://cdn.example/first.png" {
		t.Fatalf("项目卡片封面应回退到首个公开文章节点，实际 %q", cards[0].Cover)
	}

	adminCards, err := svc.ListAdminProjectCards(ctx, adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	if len(adminCards) != 1 || adminCards[0].ArticleCount != 1 ||
		adminCards[0].FallbackCover != "https://cdn.example/first.png" {
		t.Fatalf("后台项目卡片口径不对: %+v", adminCards)
	}

	detail, err := svc.ProjectDetail(ctx, pid, false)
	if err != nil {
		t.Fatalf("读项目详情失败: %v", err)
	}
	if detail.ArticleCount != 1 || detail.Cover != "https://cdn.example/first.png" {
		t.Fatalf("项目详情与卡片不一致: count=%d cover=%q", detail.ArticleCount, detail.Cover)
	}
}

// TestProjectCardHonorsShowNonAdminSwitch 项目卡片同样受「显示非管理员发布文章」
// 开关约束：不可见的文章不计入篇数，也不参与封面回退。
func TestProjectCardHonorsShowNonAdminSwitch(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	repos.FakeUsers().Seed(app.AuthorView{ID: domain.UserID(admin), Admin: true})

	pid := createProject(t, svc, "作者混合项目")
	normal := seedCardArticle(t, repos, 51, author, domain.StatusPublished, domain.VisibilityVisible,
		"https://cdn.example/normal.png")
	adminAuthored := seedCardArticle(t, repos, 52, domain.UserID(admin), domain.StatusPublished,
		domain.VisibilityVisible, "https://cdn.example/admin.png")
	seedCardArticleNode(t, repos, outline.OwnerProject, outline.ID(pid), 1, 0, normal.Uint64(), 1)
	seedCardArticleNode(t, repos, outline.OwnerProject, outline.ID(pid), 2, 0, adminAuthored.Uint64(), 2)

	cards, total, err := svc.ListProjectCards(ctx, true, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 1 || cards[0].ArticleCount != 1 || cards[0].Cover != "https://cdn.example/admin.png" {
		t.Fatalf("开关关闭时项目卡片不对: total=%d cards=%+v", total, cards)
	}

	// 只有一个非管理员作者文章的卡片，开关关闭时整张消失。
	onlyNormal := createProject(t, svc, "只有普通作者")
	seedCardArticleNode(t, repos, outline.OwnerProject, outline.ID(onlyNormal), 9, 0, normal.Uint64(), 1)
	cards, total, err = svc.ListProjectCards(ctx, true, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != pid {
		t.Fatalf("开关关闭时无公开文章的项目应消失，实际 total=%d cards=%+v", total, cards)
	}
}
