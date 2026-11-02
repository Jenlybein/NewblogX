//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是合集（collection_models / collection_article_models）的持久化契约。
//
// 只跑真 MySQL：要验的正是 SQL 行为——展示开关过滤、软删行恢复后不撞唯一索引、
// 序号压实、JOIN 过滤失效文章、封面回退。SQLite 上这些行为不一样（假 Safe）。

var collectionBaseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// migrateCollections 建出合集测试需要的表。
//
// 比其它仓储用例多带 schema.Collection：中间表有指向 collection_models 与
// article_models 的外键，两张表都得在。
func migrateCollections(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection,
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
	schema.AssertColumns(t, db, schema.CollectionArticleRow{})
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

// TestCollectionRepo_AppendItemRestoresSoftDeleted 移除后再收录恢复原行，不撞唯一索引。
func TestCollectionRepo_AppendItemRestoresSoftDeleted(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	seedCollectionArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "")
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "主题合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}

	created, err := repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(1))
	if err != nil {
		t.Fatalf("收录失败: %v", err)
	}
	if !created {
		t.Fatal("首次收录应返回 created=true")
	}

	// 重复收录：幂等 false，不产生第二行。
	created, err = repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(1))
	if err != nil {
		t.Fatalf("重复收录不应报错: %v", err)
	}
	if created {
		t.Fatal("重复收录应返回 created=false")
	}

	// 移除后再收录：唯一索引仍在（软删行占着键），必须恢复原行而不是新插入。
	if err := repo.RemoveItem(ctx, ctype.NewID(1), ctype.NewID(1)); err != nil {
		t.Fatalf("移除失败: %v", err)
	}
	created, err = repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(1))
	if err != nil {
		t.Fatalf("移除后重新收录失败（很可能撞了 uk_collection_article）: %v", err)
	}
	if !created {
		t.Fatal("恢复软删行应返回 created=true")
	}

	var n int64
	if err := db.Table("collection_article_models").Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应收敛成 1 行，实际 %d", n)
	}
}

// TestCollectionRepo_RemoveCompactsOrder 移除后剩余序号压实成 1..n。
func TestCollectionRepo_RemoveCompactsOrder(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	for _, id := range []uint64{1, 2, 3} {
		seedCollectionArticle(t, db, id, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "")
	}
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "主题合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	for _, id := range []uint64{1, 2, 3} {
		if _, err := repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(id)); err != nil {
			t.Fatalf("收录 %d 失败: %v", id, err)
		}
	}

	if err := repo.RemoveItem(ctx, ctype.NewID(1), ctype.NewID(2)); err != nil {
		t.Fatalf("移除失败: %v", err)
	}

	ids, err := repo.ListItemIDs(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 2 || ids[0].Uint64() != 1 || ids[1].Uint64() != 3 {
		t.Fatalf("移除后应剩 1,3，实际 %v", ids)
	}

	items, err := repo.ListAdminItems(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("读后台行失败: %v", err)
	}
	if items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应压实为 1,2，实际 %d,%d", items[0].SortOrder, items[1].SortOrder)
	}
}

// TestCollectionRepo_ListIncludedFiltersStale 读者侧只看到仍公开的文章，且保持顺序。
//
// 失效行**保留在表里**（后台要能看见并清理），这是与热门置顶逐字一致的口径。
func TestCollectionRepo_ListIncludedFiltersStale(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	seedCollectionArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/1.png")
	seedCollectionArticle(t, db, 2, 100, schema.ArticleStatusDraft, schema.VisibilityVisible, "")
	seedCollectionArticle(t, db, 3, 100, schema.ArticleStatusPublished, schema.VisibilityUserHidden, "")
	seedCollectionArticle(t, db, 4, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "")

	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "主题合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	for _, id := range []uint64{1, 2, 3, 4} {
		if _, err := repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(id)); err != nil {
			t.Fatalf("收录 %d 失败: %v", id, err)
		}
	}

	// 让第 4 篇后来变成私有：行保留，读者侧不出现。
	if err := db.Model(&schema.ArticleRow{}).Where("id = ?", 4).
		Update("publish_status", schema.ArticleStatusDraft).Error; err != nil {
		t.Fatalf("改状态失败: %v", err)
	}

	got, err := repo.ListIncludedArticles(ctx, ctype.NewID(1), false)
	if err != nil {
		t.Fatalf("读详情失败: %v", err)
	}
	if len(got) != 1 || got[0].ArticleID.Uint64() != 1 {
		t.Fatalf("只应剩下仍公开的文章 1，实际 %+v", got)
	}
	if got[0].Title != "合集候选 1" {
		t.Fatalf("标题未带出: %q", got[0].Title)
	}

	adminItems, err := repo.ListAdminItems(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("读后台行失败: %v", err)
	}
	if len(adminItems) != 4 {
		t.Fatalf("后台应仍看到 4 行（失效行保留），实际 %d", len(adminItems))
	}
	if !adminItems[1].Stale() {
		t.Fatal("草稿文章的后台行应标为失效")
	}
	if adminItems[0].Stale() {
		t.Fatal("仍公开的文章不应标为失效")
	}
}

// TestCollectionRepo_ListCardsCoverFallback 没有封面的合集退回第一篇公开文章的封面。
func TestCollectionRepo_ListCardsCoverFallback(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	seedCollectionArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://img/1.png")

	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	// 主表 cover 留空（domain 允许），期望读模型回退到首篇封面。
	c := newCollection(t, 1, "没封面的合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	if _, err := repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(1)); err != nil {
		t.Fatalf("收录失败: %v", err)
	}

	cards, total, err := repo.ListCards(ctx, false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读合集列表失败: %v", err)
	}
	if total != 1 || len(cards) != 1 {
		t.Fatalf("应有 1 个合集，实际 total=%d len=%d", total, len(cards))
	}
	if cards[0].Cover != "https://img/1.png" {
		t.Fatalf("封面应回退到首篇公开文章的封面，实际 %q", cards[0].Cover)
	}
	if cards[0].ArticleCount != 1 {
		t.Fatalf("收录数应为 1，实际 %d", cards[0].ArticleCount)
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

// TestCollectionRepo_UpdateItemOrder 全集顺序替换真的落库。
func TestCollectionRepo_UpdateItemOrder(t *testing.T) {
	t.Parallel()

	db := migrateCollections(t)
	seedUser(t, db, 100)
	for _, id := range []uint64{1, 2} {
		seedCollectionArticle(t, db, id, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "")
	}
	repo := mysql.NewCollectionRepo(db)
	ctx := context.Background()

	c := newCollection(t, 1, "主题合集", true, 1)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	for _, id := range []uint64{1, 2} {
		if _, err := repo.AppendItem(ctx, ctype.NewID(1), ctype.NewID(id)); err != nil {
			t.Fatalf("收录失败: %v", err)
		}
	}

	if err := repo.UpdateItemOrder(ctx, ctype.NewID(1),
		[]collection.ArticleID{ctype.NewID(2), ctype.NewID(1)}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}

	got, err := repo.ListIncludedArticles(ctx, ctype.NewID(1), false)
	if err != nil {
		t.Fatalf("读详情失败: %v", err)
	}
	if len(got) != 2 || got[0].ArticleID.Uint64() != 2 || got[0].SortOrder != 1 {
		t.Fatalf("重排后第一篇应为 2 且序号 1，实际 %+v", got)
	}
}

// isNotFound 判断是不是"合集不存在"。
func isNotFound(err error) bool {
	return err == collection.ErrNotFound
}
