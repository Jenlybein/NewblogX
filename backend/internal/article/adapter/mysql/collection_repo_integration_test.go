//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是合集（collection_models）的持久化契约。
//
// 只跑真 MySQL：要验的正是 SQL 行为——展示开关过滤、软删落库、卡片篇数与
// 封面回退（都从目录树派生）。SQLite 上这些行为不一样（假 Safe）。
//
// 旧扁平收录中间表的仓储用例随表一起退役（#225）：收录与层次只经目录树。

// isNotFound 判断仓储错误是否为"合集不存在"（域错误按码比较）。
func isNotFound(err error) bool {
	return errors.Is(err, collection.ErrNotFound)
}

var collectionBaseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// migrateCollections 建出合集测试需要的表。
//
// 比其它仓储用例多带 schema.Collection 与 schema.Outline：卡片读面只认目录树。
func migrateCollections(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection, schema.Outline,
	)
	return db
}

// seedCollectionArticle 插入一篇候选文章。
func seedCollectionArticle(
	t *testing.T, db *gorm.DB, id, authorID uint64,
	status schema.ArticleStatus, visibility schema.ArticleVisibility, cover string,
) {
	t.Helper()
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          "合集候选 " + itoa(id),
		Abstract:       "摘要" + itoa(id),
		Content:        "正文",
		AuthorID:       authorID,
		ViewCount:      int(id * 10),
		CommentsToggle: true,
		PublishStatus:  status,
		Visibility:     visibility,
		CreatedAt:      collectionBaseTime,
		UpdatedAt:      collectionBaseTime,
	})
	// cover 不进 ArticleSpec（它只列测试真用到的字段），需要时显式写一次。
	if cover != "" {
		if err := db.Model(&schema.ArticleRow{}).Where("id = ?", id).
			Update("cover", cover).Error; err != nil {
			t.Fatalf("写封面失败: %v", err)
		}
	}
}

// seedCollectionTreeNode 往合集树里播种一行（卡片读面只认目录树）。
func seedCollectionTreeNode(
	t *testing.T, db *gorm.DB, collectionID uint64, spec schema.OutlineNodeSpec,
) {
	t.Helper()
	spec.OwnerKind = int(outline.OwnerCollection)
	spec.OwnerID = collectionID
	schema.InsertOutlineNode(t, db, spec)
}

// newCollection 建一个聚合（绕过用例，直接测仓储）。
func newCollection(t *testing.T, id uint64, title string, displayed bool, order int) *collection.Collection {
	t.Helper()
	c, err := collection.New(collection.NewParams{
		ID:        ctype.NewID(id),
		Title:     title,
		CreatorID: ctype.NewID(100),
		Displayed: displayed,
		SortOrder: order,
		Now:       collectionBaseTime,
	})
	if err != nil {
		t.Fatalf("构造合集失败: %v", err)
	}
	return c
}

// TestCollectionRowsMatchSchema 逐列比对：adapter 的行结构与真表列一致。
//
// 这是"新表三处登记"里最容易被漏掉的一半：schema.sql 建出来的表与 Go 行结构
// 一旦漂移，业务列会静默读不到（GORM 不会报错）。
func TestCollectionRowsMatchSchema(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	schema.AssertColumns(t, db, schema.CollectionRow{})
}

// TestCollectionRepo_CreateAndFind 新建的合集能按 ID 读回，字段逐一落地。
func TestCollectionRepo_CreateAndFind(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "Nuxt 部署实战", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}

	got, err := repo.Find(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("读合集失败: %v", err)
	}
	if got.Title() != "Nuxt 部署实战" || !got.Displayed() || got.SortOrder() != 1 {
		t.Fatalf("读回的合集字段不对: %+v", got)
	}

	if _, err := repo.Find(ctx, ctype.NewID(999)); !isNotFound(err) {
		t.Fatalf("不存在的合集应返回 ErrNotFound，实际 %v", err)
	}
}

// TestCollectionRepo_UpdateDisplay 关闭展示要真的落库（false 不能被零值跳过）。
func TestCollectionRepo_UpdateDisplay(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "还没发布的专题", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}

	c.SetDisplay(false, collectionBaseTime)
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("关闭展示失败: %v", err)
	}

	got, err := repo.Find(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("读合集失败: %v", err)
	}
	if got.Displayed() {
		t.Fatal("关闭展示后 is_display 应落成 false（GORM 零值跳过是这个坑的典型现场）")
	}
}

// TestCollectionRepo_ListCardsCoverFallback 没有封面的合集退回目录树里第一个
// 公开文章节点的封面（旧中间表为空也不影响）。
func TestCollectionRepo_ListCardsCoverFallback(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	seedCollectionArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/1.png")

	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	// 主表 cover 留空（domain 允许），期望读模型回退到首个公开文章节点。
	c := newCollection(t, 1, "没封面的合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 1, Kind: int(outline.NodeKindArticle), ArticleID: 1,
		SortOrder: 1, At: collectionBaseTime,
	})

	cards, total, err := repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 {
		t.Fatalf("应有 1 个合集，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].Cover != "https://img/1.png" {
		t.Fatalf("封面应回退到首个公开文章节点的封面，实际 %q", cards[0].Cover)
	}
	if cards[0].ArticleCount != 1 {
		t.Fatalf("篇数应为 1，实际 %d", cards[0].ArticleCount)
	}
}

// TestCollectionRepo_ListCardsHidesUndisplayed 未展示的合集不进读者列表，后台仍可见。
func TestCollectionRepo_ListCardsHidesUndisplayed(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	shown := newCollection(t, 1, "已发布专题", true, 1)
	hidden := newCollection(t, 2, "还没发布的专题", false, 2)
	for _, c := range []*collection.Collection{shown, hidden} {
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("建合集失败: %v", err)
		}
	}

	cards, total, err := repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID.Uint64() != 1 {
		t.Fatalf("只应出现展示中的合集 1，实际 total=%d cards=%+v", total, cards)
	}

	adminCards, err := repo.ListAdminCards(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(adminCards) != 2 {
		t.Fatalf("后台应看到 2 个合集，实际 %d", len(adminCards))
	}
	if adminCards[0].SortOrder != 1 || adminCards[1].SortOrder != 2 {
		t.Fatalf("后台顺序号应从 1 连续编号，实际 %d,%d",
			adminCards[0].SortOrder, adminCards[1].SortOrder)
	}
}

// TestCollectionRepo_ListAdminCardsFallbackCover 后台卡片单独给出回退封面，
// 但主表 cover 保持空串：编辑表单要能区分"我传过封面"与"这是回退来的"。
//
// 篇数与回退封面与读者卡片同一口径（目录树），失效行不进数。
func TestCollectionRepo_ListAdminCardsFallbackCover(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	seedCollectionArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/1.png")

	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "没封面的合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 1, Kind: int(outline.NodeKindArticle), ArticleID: 1,
		SortOrder: 1, At: collectionBaseTime,
	})

	cards, err := repo.ListAdminCards(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("应有 1 个合集，实际 %d", len(cards))
	}
	if cards[0].Cover != "" {
		t.Fatalf("没上传封面时 cover 应保持空串，实际 %q", cards[0].Cover)
	}
	if cards[0].FallbackCover != "https://img/1.png" {
		t.Fatalf("回退封面应取首个公开文章节点的封面，实际 %q", cards[0].FallbackCover)
	}
	if cards[0].ArticleCount != 1 {
		t.Fatalf("后台篇数应与读者同口径（1），实际 %d", cards[0].ArticleCount)
	}
}

// TestCollectionRepo_CardsReadOutlineTree 卡片篇数与封面回退只认目录树：
// 树里有公开文章、旧中间表为空时卡片仍显示 N 篇；封面取 DFS 第一个公开
// 文章节点（为空不借用后一篇）；onlyAdminAuthors 与后台卡片同口径。
func TestCollectionRepo_CardsReadOutlineTree(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	schema.InsertUser(t, db, schema.UserSpec{
		ID: 100, Username: "normal", Nickname: "普通作者", Role: schema.RoleUser,
	})
	schema.InsertUser(t, db, schema.UserSpec{
		ID: 900, Username: "admin", Nickname: "管理员", Role: schema.RoleAdmin,
	})
	seedCollectionArticle(t, db, 11, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/first.png")
	seedCollectionArticle(t, db, 12, 900, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/second.png")
	seedCollectionArticle(t, db, 13, 100, schema.ArticleStatusDraft, schema.VisibilityVisible, "https://img/draft.png")
	seedCollectionArticle(t, db, 14, 100, schema.ArticleStatusPublished, schema.VisibilityUserHidden, "")

	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	// 合集 1 无封面，树是「第一章 → 11、12」+ 根层 13（草稿）/14（隐藏）。
	if err := repo.Create(ctx, newCollection(t, 1, "目录树合集", true, 1)); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 10, Kind: int(outline.NodeKindFolder), Title: "第一章", SortOrder: 1, At: collectionBaseTime,
	})
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 11, ParentID: 10, Kind: int(outline.NodeKindArticle), ArticleID: 11,
		SortOrder: 1, At: collectionBaseTime,
	})
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 12, ParentID: 10, Kind: int(outline.NodeKindArticle), ArticleID: 12,
		SortOrder: 2, At: collectionBaseTime,
	})
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 13, Kind: int(outline.NodeKindArticle), ArticleID: 13, SortOrder: 2, At: collectionBaseTime,
	})
	seedCollectionTreeNode(t, db, 1, schema.OutlineNodeSpec{
		ID: 14, Kind: int(outline.NodeKindArticle), ArticleID: 14, SortOrder: 3, At: collectionBaseTime,
	})

	// 合集 2 有显式封面，树里只挂 12。
	if err := repo.Create(ctx, newCollection(t, 2, "显式封面合集", true, 2)); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	if err := db.Table("collection_models").Where("id = ?", 2).
		Update("cover", "https://img/explicit.png").Error; err != nil {
		t.Fatalf("写合集封面失败: %v", err)
	}
	seedCollectionTreeNode(t, db, 2, schema.OutlineNodeSpec{
		ID: 20, Kind: int(outline.NodeKindArticle), ArticleID: 12, SortOrder: 1, At: collectionBaseTime,
	})

	cards, total, err := repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 2 || len(cards) != 2 {
		t.Fatalf("应有 2 个合集卡片，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].ArticleCount != 2 {
		t.Fatalf("合集 1 的篇数应只数树里公开的 11、12（2），实际 %d", cards[0].ArticleCount)
	}
	if cards[0].Cover != "https://img/first.png" {
		t.Fatalf("封面应回退到 DFS 第一个公开文章节点（11），实际 %q", cards[0].Cover)
	}
	if cards[1].ArticleCount != 1 || cards[1].Cover != "https://img/explicit.png" {
		t.Fatalf("合集 2 应优先用显式封面、篇数 1，实际 %+v", cards[1])
	}

	// 首节点没有封面时最终封面为空：不能借用后一篇（12）的 second.png。
	if err := db.Table("article_models").Where("id = ?", 11).Update("cover", "").Error; err != nil {
		t.Fatalf("清空文章封面失败: %v", err)
	}
	cards, _, err = repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if cards[0].Cover != "" {
		t.Fatalf("第一个公开文章节点没有封面时不应借用后一篇，实际 %q", cards[0].Cover)
	}

	// 开关关闭：非管理员作者的文章不计入篇数、不参与封面回退。
	cards, _, err = repo.ListCards(ctx, true, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("两个合集都仍有管理员作者的文章，实际 %+v", cards)
	}
	if cards[0].ArticleCount != 1 || cards[0].Cover != "https://img/second.png" {
		t.Fatalf("开关关闭时合集 1 应只剩管理员作者的 12，实际 %+v", cards[0])
	}

	// 后台卡片同口径：只算公开文章，回退封面取 DFS 首个公开节点。
	adminCards, err := repo.ListAdminCards(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(adminCards) != 2 {
		t.Fatalf("后台应有 2 个合集，实际 %d", len(adminCards))
	}
	if adminCards[0].ArticleCount != 2 || adminCards[0].Cover != "" {
		t.Fatalf("后台合集 1 篇数应同读者口径（2）、cover 保持空串，实际 %+v", adminCards[0])
	}
	if adminCards[1].ArticleCount != 1 || adminCards[1].Cover != "https://img/explicit.png" {
		t.Fatalf("后台合集 2 不对: %+v", adminCards[1])
	}
}

// TestCollectionRepo_UpdatePersistsSoftDelete 软删必须真的写进 deleted_at：
// 否则后台列表照旧出现、读者侧也还能按 ID 打开。
func TestCollectionRepo_UpdatePersistsSoftDelete(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "要下线的专题", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}

	c.SoftDelete(collectionBaseTime.Add(time.Hour))
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	if _, err := repo.Find(ctx, ctype.NewID(1)); !isNotFound(err) {
		t.Fatalf("软删后的合集应读不到，实际 %v", err)
	}

	cards, total, err := repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读读者列表失败: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("软删的合集不该出现在读者列表，实际 total=%d cards=%+v", total, cards)
	}

	adminCards, err := repo.ListAdminCards(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(adminCards) != 0 {
		t.Fatalf("软删的合集不该出现在后台列表，实际 %+v", adminCards)
	}
}
