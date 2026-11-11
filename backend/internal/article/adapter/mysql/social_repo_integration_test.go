//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是「显示非管理员发布文章」开关在**查询层**的契约测试（ADR-0017 决策 4）。
//
// 为什么必须真 MySQL：过滤是 SQL（join user_models 判 role），而且 join 必须
// 同时作用于 count 与当页查询。用内存 fake 只验证"开关传下去了"，验证不了
// 这两件事，也验证不了"作者改了角色之后过滤立刻跟着变"。

const (
	visNormalAuthor uint64 = 100
	visAdminAuthor  uint64 = 900
	visReader       uint64 = 200
)

// TestFavoriteItemsFiltersNonAdminAuthors 验证收藏内容的读过滤。
func TestFavoriteItemsFiltersNonAdminAuthors(t *testing.T) {
	db := migrate(t)
	seedVisibilityUsers(t, db)
	seedVisibilityArticles(t, db)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	schema.InsertFavorite(t, db, schema.FavoriteSpec{
		ID: 1, UserID: visReader, Title: "默认收藏夹", IsDefault: true, ArticleCount: 2, At: at,
	})
	for k, item := range []struct {
		articleID, authorID ctype.ID
		title               string
	}{
		{articleID: ctype.NewID(1001), authorID: ctype.NewID(visNormalAuthor), title: "普通作者的文章"},
		{articleID: ctype.NewID(1002), authorID: ctype.NewID(visAdminAuthor), title: "管理员作者的文章"},
	} {
		schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
			ID:              uint64(5000 + k),
			ArticleID:       item.articleID.Uint64(),
			UserID:          visReader,
			FavorID:         1,
			ArticleTitle:    item.title,
			ArticleStatus:   schema.ArticleStatusPublished,
			ArticleAuthorID: item.authorID.Uint64(),
			At:              at,
		})
	}

	repo := mysql.NewFavoriteRepo(db)
	ctx := context.Background()

	// 开关打开：两篇都在，total 也是 2。
	items, total, err := repo.ListItems(ctx, ctype.NewID(1), 0, 10, false, false)
	if err != nil {
		t.Fatalf("列收藏内容失败: %v", err)
	}
	if len(items) != 2 || total != 2 {
		t.Fatalf("开关打开时应有 2 条，实际 list=%d total=%d", len(items), total)
	}

	// 开关关闭：只剩管理员作者那篇，且 total 必须一起变小（否则前端分页错乱）。
	items, total, err = repo.ListItems(ctx, ctype.NewID(1), 0, 10, true, false)
	if err != nil {
		t.Fatalf("列收藏内容（带过滤）失败: %v", err)
	}
	if len(items) != 1 || total != 1 {
		t.Fatalf("开关关闭时应有 1 条，实际 list=%d total=%d", len(items), total)
	}
	if items[0].ArticleID() != domain.ArticleID(ctype.NewID(1002)) {
		t.Fatalf("应只剩管理员作者的文章，实际 %s", items[0].ArticleID())
	}

	// 角色是现读的：管理员作者被降级之后，同一份数据在过滤下也读不出来。
	if err := db.Exec("UPDATE user_models SET role = ? WHERE id = ?",
		int8(schema.RoleUser), visAdminAuthor).Error; err != nil {
		t.Fatalf("修改作者角色失败: %v", err)
	}
	items, total, err = repo.ListItems(ctx, ctype.NewID(1), 0, 10, true, false)
	if err != nil {
		t.Fatalf("列收藏内容（降级后）失败: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Fatalf("作者降级后过滤结果应为空，实际 list=%d total=%d", len(items), total)
	}
}

// TestViewHistoryFiltersNonAdminAuthors 验证浏览历史的读过滤。
//
// 历史表里没有作者快照，作者角色要经由 article_models 现取 —— 这条路径
// 与收藏内容不同，写错了只会表现为"历史少了几条"，所以单独测。
//
// 过滤必须发生在分页**之前**：过滤掉的最新一条不能把当页的第一条挤掉
// （否则 limit=1 会返回空页，而总数里还有内容）。这里用 limit=1 验证这一点。
func TestViewHistoryFiltersNonAdminAuthors(t *testing.T) {
	db := migrate(t)
	seedVisibilityUsers(t, db)
	seedVisibilityArticles(t, db)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// 非管理员作者那篇更晚（排序上排第一）——开关关闭时它必须被过滤掉，
	// 而不是占掉唯一的一个名额（这样 limit=1 也不会返回空页）。
	schema.InsertViewHistory(t, db, 6001, visReader, 1002, at)
	schema.InsertViewHistory(t, db, 6002, visReader, 1001, at.Add(time.Minute))

	repo := mysql.NewViewHistoryRepo(db)
	ctx := context.Background()

	records, err := repo.List(ctx, viewhistory.ListQuery{
		UserID: domain.UserID(ctype.NewID(visReader)),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("列浏览历史失败: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("开关打开时应有 2 条，实际 %d", len(records))
	}

	records, err = repo.List(ctx, viewhistory.ListQuery{
		UserID:           domain.UserID(ctype.NewID(visReader)),
		OnlyAdminAuthors: true,
		Limit:            1,
	})
	if err != nil {
		t.Fatalf("列浏览历史（带过滤）失败: %v", err)
	}
	if len(records) != 1 || records[0].ArticleID() != domain.ArticleID(ctype.NewID(1002)) {
		t.Fatalf("开关关闭时 limit=1 应返回管理员作者那篇，实际 %v", records)
	}
}

// seedVisibilityUsers 插入普通作者 / 管理员作者 / 读者三个用户。
func seedVisibilityUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, spec := range []struct {
		id       uint64
		username string
		nickname string
		role     schema.RoleType
	}{
		{id: visNormalAuthor, username: "vis_author", nickname: "普通作者", role: schema.RoleUser},
		{id: visAdminAuthor, username: "vis_admin", nickname: "管理员作者", role: schema.RoleAdmin},
		{id: visReader, username: "vis_reader", nickname: "读者", role: schema.RoleUser},
	} {
		schema.InsertUser(t, db, schema.UserSpec{
			ID:       spec.id,
			Username: spec.username,
			Nickname: spec.nickname,
			Role:     spec.role,
		})
	}
}

// seedVisibilityArticles 插入两篇已发布可见的文章（分别属于普通作者与管理员作者）。
func seedVisibilityArticles(t *testing.T, db *gorm.DB) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, spec := range []struct {
		id     uint64
		author uint64
		title  string
	}{
		{id: 1001, author: visNormalAuthor, title: "普通作者的文章"},
		{id: 1002, author: visAdminAuthor, title: "管理员作者的文章"},
	} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             spec.id,
			Title:          spec.title,
			AuthorID:       spec.author,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      at,
			UpdatedAt:      at,
		})
	}
}
