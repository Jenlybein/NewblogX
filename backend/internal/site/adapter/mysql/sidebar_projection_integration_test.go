//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/site/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是右栏「推荐合集」数据源（站点模块自己的只读投影查询）的持久化契约。
//
// 只跑真 MySQL：要验的正是 SQL 行为——只取展示中的合集、文章数只数仍公开的文章、
// 封面回退、合集自己的顺序、条数上限。这些过滤都在 SQL 里，前端与用例层都不重判。

var sidebarCollectionBaseTime = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// migrateSidebarCollections 建出合集投影读需要的表（中间表有两个外键）。
func migrateSidebarCollections(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Collection)
	return db
}

// seedSidebarCollection 插入一个合集。
//
// 用 map 插入而不是结构体：is_display 带 default:1，GORM 的结构体 Insert 会跳过
// 零值字段，把"不展示的合集"静默写成展示 —— 这条用例要造的正是那种行。
func seedSidebarCollection(
	t *testing.T, db *gorm.DB, id uint64, title string, displayed bool, order int, cover string,
) {
	t.Helper()
	err := db.Model(&schema.CollectionRow{}).Create(map[string]any{
		"id":          ctype.NewID(id),
		"created_at":  sidebarCollectionBaseTime,
		"updated_at":  sidebarCollectionBaseTime,
		"title":       title,
		"description": "简介：" + title,
		"cover":       cover,
		"is_display":  displayed,
		"sort_order":  order,
		"creator_id":  ctype.NewID(100),
	}).Error
	if err != nil {
		t.Fatalf("插入合集 %d 失败: %v", id, err)
	}
}

// seedSidebarArticle 插入一篇候选文章（状态与封面可控）。
func seedSidebarArticle(
	t *testing.T, db *gorm.DB, id, authorID uint64,
	status schema.ArticleStatus, visibility schema.ArticleVisibility, cover string,
) {
	t.Helper()
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          "合集候选 " + itoa(id),
		Abstract:       "摘要",
		Content:        "正文",
		AuthorID:       authorID,
		CommentsToggle: true,
		PublishStatus:  status,
		Visibility:     visibility,
		CreatedAt:      sidebarCollectionBaseTime,
		UpdatedAt:      sidebarCollectionBaseTime,
	})
	if cover != "" {
		if err := db.Model(&schema.ArticleRow{}).Where("id = ?", id).
			Update("cover", cover).Error; err != nil {
			t.Fatalf("写封面失败: %v", err)
		}
	}
}

// seedSidebarCollectionItem 把一篇文章挂进合集（顺序可控）。
func seedSidebarCollectionItem(
	t *testing.T, db *gorm.DB, id, collectionID, articleID uint64, order int,
) {
	t.Helper()
	row := schema.CollectionArticleRow{
		BaseRow: schema.BaseRow{
			ID:        ctype.NewID(id),
			CreatedAt: sidebarCollectionBaseTime,
			UpdatedAt: sidebarCollectionBaseTime,
		},
		CollectionID: ctype.NewID(collectionID),
		ArticleID:    ctype.NewID(articleID),
		SortOrder:    order,
	}
	if err := db.Omit("CollectionModel", "ArticleModel").Create(&row).Error; err != nil {
		t.Fatalf("插入合集收录行失败: %v", err)
	}
}

// itoa 把 uint64 转成十进制字符串（测试标题用）。
func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// 只取展示中的合集；顺序取合集自己的 sort_order；软删的合集不出现。
func TestSidebarDataCollectionsOnlyDisplayed(t *testing.T) {
	t.Parallel()

	db := migrateSidebarCollections(t)
	seedSidebarCollection(t, db, 1, "部署实战", true, 2, "")
	seedSidebarCollection(t, db, 2, "还没公布的专题", false, 0, "")
	seedSidebarCollection(t, db, 3, "从零搭博客", true, 1, "")
	seedSidebarCollection(t, db, 4, "已删除的合集", true, 3, "")
	if err := db.Model(&schema.CollectionRow{}).Where("id = ?", ctype.NewID(4)).
		Update("deleted_at", sidebarCollectionBaseTime).Error; err != nil {
		t.Fatalf("软删合集失败: %v", err)
	}

	got, err := sitemysql.NewSidebarData(db).Collections(context.Background(), domain.SidebarCardDataLimit)
	if err != nil {
		t.Fatalf("读取推荐合集失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("未展示 / 已删除的合集都不该返回：期望 2 个，实际 %+v", got)
	}
	if got[0].Title != "从零搭博客" || got[1].Title != "部署实战" {
		t.Fatalf("应按合集自己的 sort_order 升序返回: %+v", got)
	}
	if got[0].Description != "简介：从零搭博客" {
		t.Fatalf("简介应原样投影: %+v", got[0])
	}
}

// 文章数只数仍公开的文章；封面为空时回退到首篇公开文章的封面。
func TestSidebarDataCollectionsCountAndCover(t *testing.T) {
	t.Parallel()

	db := migrateSidebarCollections(t)
	schema.InsertUser(t, db, schema.UserSpec{ID: 100, Username: "admin", Nickname: "管理员"})
	seedSidebarCollection(t, db, 1, "从零搭博客", true, 1, "")
	seedSidebarCollection(t, db, 2, "有封面的合集", true, 2, "https://cdn.example.com/collection.png")

	seedSidebarArticle(t, db, 1, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://cdn.example.com/first.png")
	seedSidebarArticle(t, db, 2, 100, schema.ArticleStatusPublished, schema.VisibilityVisible, "https://cdn.example.com/second.png")
	seedSidebarArticle(t, db, 3, 100, schema.ArticleStatusDraft, schema.VisibilityVisible, "")
	seedSidebarArticle(t, db, 4, 100, schema.ArticleStatusPublished, schema.VisibilityAdminHidden, "")

	// 合集 1 的收录顺序：草稿(1)、隐藏(2)、公开一(3)、公开二(4)。
	// 首个**公开**条目是文章 1，所以封面回退到 first.png；文章数是 2（不是 4）。
	seedSidebarCollectionItem(t, db, 11, 1, 3, 1)
	seedSidebarCollectionItem(t, db, 12, 1, 4, 2)
	seedSidebarCollectionItem(t, db, 13, 1, 1, 3)
	seedSidebarCollectionItem(t, db, 14, 1, 2, 4)
	// 合集 2：只收录一篇公开文章，封面用合集自己的。
	seedSidebarCollectionItem(t, db, 21, 2, 1, 1)

	got, err := sitemysql.NewSidebarData(db).Collections(context.Background(), domain.SidebarCardDataLimit)
	if err != nil {
		t.Fatalf("读取推荐合集失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 个合集，实际 %+v", got)
	}
	if got[0].ArticleCount != 2 {
		t.Fatalf("文章数只数仍公开的文章：期望 2，实际 %d（%+v）", got[0].ArticleCount, got[0])
	}
	if got[0].Cover != "https://cdn.example.com/first.png" {
		t.Fatalf("没有封面时应回退到首篇公开文章的封面，实际 %q", got[0].Cover)
	}
	if got[1].Cover != "https://cdn.example.com/collection.png" {
		t.Fatalf("合集自己的封面优先，实际 %q", got[1].Cover)
	}
	if got[1].ArticleCount != 1 {
		t.Fatalf("合集 2 的文章数应为 1，实际 %d", got[1].ArticleCount)
	}
}

// 一次性最多回传一张侧栏卡能放下的条数（与其它种类同一个上限）。
func TestSidebarDataCollectionsLimit(t *testing.T) {
	t.Parallel()

	db := migrateSidebarCollections(t)
	for i := 1; i <= 6; i++ {
		seedSidebarCollection(t, db, uint64(i), "合集"+itoa(uint64(i)), true, i, "")
	}

	got, err := sitemysql.NewSidebarData(db).Collections(context.Background(), 3)
	if err != nil {
		t.Fatalf("读取推荐合集失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("应按 limit 截断：期望 3 个，实际 %d", len(got))
	}
	if got[0].Title != "合集1" || got[2].Title != "合集3" {
		t.Fatalf("截断应按合集顺序取前 N 个: %+v", got)
	}
}
