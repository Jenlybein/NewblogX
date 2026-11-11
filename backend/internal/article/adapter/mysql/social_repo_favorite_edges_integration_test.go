//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 本文件守 #344 的四处收藏夹读/写口径。都必须在真 MySQL 上跑：
// 计数子查询、级联删除、has_article 的 EXISTS 与公开面 JOIN 都是 SQL 语义，
// 内存 fake 无法证伪。

const edgeReader = uint64(7_001)

// seedEdgeReader 建出本文件的读者用户：收藏夹与文章都有指向 user_models 的外键，
// 不先建用户会直接撞 FK。
func seedEdgeReader(t *testing.T, db *gorm.DB, at time.Time) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{ID: edgeReader, Username: "边缘读者", Nickname: "边缘读者", CreatedAt: at})
}

// TestFoldersExposeRealCountsAndHasArticle 锁住「N 篇」与「是否已包含」。
//
// 历史缺陷：article_count 从无写入（恒 0），has_article 恒 false ——
// 前者让用户以为没收藏进去，后者让弹窗永远显示"加入"、永远不会显示勾。
func TestFoldersExposeRealCountsAndHasArticle(t *testing.T) {
	db := migrate(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedEdgeReader(t, db, at)

	// 冗余列故意写成错值：读侧必须现算，不能信任这一列。
	schema.InsertFavorite(t, db, schema.FavoriteSpec{ID: 101, UserID: edgeReader, Title: "默认收藏", IsDefault: true, ArticleCount: 0, At: at})
	schema.InsertFavorite(t, db, schema.FavoriteSpec{ID: 102, UserID: edgeReader, Title: "第二个夹", ArticleCount: 0, At: at})

	// 收藏关系对 article_models 有外键：先把被收藏的文章建出来。
	for i := 0; i < 3; i++ {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID: uint64(8_001 + i), Title: "文章", AuthorID: edgeReader,
			PublishStatus: schema.ArticleStatusPublished, CreatedAt: at, UpdatedAt: at,
		})
	}

	for i, folderID := range []uint64{101, 102, 102} {
		schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
			ID:              uint64(9_001 + i),
			ArticleID:       uint64(8_001 + i),
			UserID:          edgeReader,
			FavorID:         folderID,
			ArticleTitle:    "文章",
			ArticleStatus:   schema.ArticleStatusPublished,
			ArticleAuthorID: edgeReader,
			At:              at,
		})
	}

	repo := mysql.NewFavoriteRepo(db)
	ctx := context.Background()

	// 不带 article_id：has_article 全为 false，但计数必须是真数。
	folders, err := repo.ListFolders(ctx, favorite.ListFoldersQuery{UserID: domain.UserID(ctype.NewID(edgeReader))})
	if err != nil {
		t.Fatalf("列收藏夹失败: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("应有 2 个收藏夹，实际 %d", len(folders))
	}
	if folders[0].ID() != favorite.ID(ctype.NewID(101)) {
		t.Fatalf("默认夹必须排第一，实际首位 %s", folders[0].ID())
	}
	if got := folders[0].ArticleCount(); got != 1 {
		t.Fatalf("默认夹应有 1 篇（冗余列是 0，必须现算），实际 %d", got)
	}
	if got := folders[1].ArticleCount(); got != 2 {
		t.Fatalf("第二个夹应有 2 篇，实际 %d", got)
	}

	// 带 article_id：只有包含它的夹 has_article 为真。
	folders, err = repo.ListFolders(ctx, favorite.ListFoldersQuery{
		UserID:    domain.UserID(ctype.NewID(edgeReader)),
		ArticleID: domain.ArticleID(ctype.NewID(8_002)),
	})
	if err != nil {
		t.Fatalf("列收藏夹（带 article_id）失败: %v", err)
	}
	has := map[string]bool{}
	for _, f := range folders {
		has[f.ID().String()] = f.HasArticle()
	}
	if !has[favorite.ID(ctype.NewID(102)).String()] {
		t.Fatalf("文章 8002 在 102 里，has_article 应为真: %v", has)
	}
	if has[favorite.ID(ctype.NewID(101)).String()] {
		t.Fatalf("文章 8002 不在 101 里，has_article 应为假: %v", has)
	}
}

// TestDeleteFolderCascadesWithoutTouchingArticles 锁住「只删分组、不动文章」。
//
// 旧实现只软删夹子行，关系行留成孤儿：文章仍显示"已收藏"，却不在任何夹子里，
// 用户永远找不到它（#344）。
func TestDeleteFolderCascadesWithoutTouchingArticles(t *testing.T) {
	db := migrate(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedEdgeReader(t, db, at)

	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 8_101, Title: "文章", AuthorID: edgeReader, PublishStatus: schema.ArticleStatusPublished,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertFavorite(t, db, schema.FavoriteSpec{ID: 201, UserID: edgeReader, Title: "待删夹", At: at})
	schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
		ID: 9_101, ArticleID: 8_101, UserID: edgeReader, FavorID: 201,
		ArticleTitle: "文章", ArticleStatus: schema.ArticleStatusPublished, ArticleAuthorID: edgeReader, At: at,
	})

	repo := mysql.NewFavoriteRepo(db)
	if err := repo.DeleteFolder(context.Background(), favorite.ID(ctype.NewID(201))); err != nil {
		t.Fatalf("删除收藏夹失败: %v", err)
	}

	// 关系行必须一起消失（软删）。
	var liveFavors int64
	if err := db.Table("user_article_favor_models").
		Where("favor_id = ? AND deleted_at IS NULL", ctype.NewID(201)).
		Count(&liveFavors).Error; err != nil {
		t.Fatalf("统计关系行失败: %v", err)
	}
	if liveFavors != 0 {
		t.Fatalf("删夹后不应留下活跃关系行，实际 %d", liveFavors)
	}

	// 文章本身必须还在（且未被软删）。
	var liveArticles int64
	if err := db.Table("article_models").
		Where("id = ? AND deleted_at IS NULL", ctype.NewID(8_101)).
		Count(&liveArticles).Error; err != nil {
		t.Fatalf("统计文章失败: %v", err)
	}
	if liveArticles != 1 {
		t.Fatalf("删夹不得删除文章，实际剩下 %d 篇", liveArticles)
	}
}

// TestRefreshFolderStatsRepairsStaleColumn 锁住写侧重算：列被写坏后能自愈，
// 且夹子的 updated_at 会随之推进（列表按它排序）。
func TestRefreshFolderStatsRepairsStaleColumn(t *testing.T) {
	db := migrate(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedEdgeReader(t, db, at)

	schema.InsertFavorite(t, db, schema.FavoriteSpec{ID: 301, UserID: edgeReader, Title: "夹", ArticleCount: 99, At: at})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 8_301, Title: "文章", AuthorID: edgeReader,
		PublishStatus: schema.ArticleStatusPublished, CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
		ID: 9_301, ArticleID: 8_301, UserID: edgeReader, FavorID: 301,
		ArticleTitle: "文章", ArticleStatus: schema.ArticleStatusPublished, ArticleAuthorID: edgeReader, At: at,
	})

	bumped := at.Add(time.Hour)
	repo := mysql.NewFavoriteRepo(db)
	if err := repo.RefreshFolderStats(context.Background(), favorite.ID(ctype.NewID(301)), bumped); err != nil {
		t.Fatalf("刷新统计失败: %v", err)
	}

	var row struct {
		ArticleCount int
		UpdatedAt    time.Time
	}
	if err := db.Table("favorite_models").Select("article_count, updated_at").
		Where("id = ?", ctype.NewID(301)).Take(&row).Error; err != nil {
		t.Fatalf("读回收藏夹失败: %v", err)
	}
	if row.ArticleCount != 1 {
		t.Fatalf("重算后应为 1 篇（原值是错的 99），实际 %d", row.ArticleCount)
	}
	if !row.UpdatedAt.Equal(bumped) {
		t.Fatalf("updated_at 应被推进到 %s，实际 %s", bumped, row.UpdatedAt)
	}
}

// TestListItemsPublishedOnlyHidesUnpublished 锁住公开读面只列已发布。
//
// 收藏时的快照状态会过期：文章之后被撤回草稿，公开主页不该继续展示它。
func TestListItemsPublishedOnlyHidesUnpublished(t *testing.T) {
	db := migrate(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedEdgeReader(t, db, at)

	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 8_401, Title: "已发布", AuthorID: edgeReader, PublishStatus: schema.ArticleStatusPublished,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 8_402, Title: "草稿", AuthorID: edgeReader, PublishStatus: schema.ArticleStatusDraft,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertFavorite(t, db, schema.FavoriteSpec{ID: 401, UserID: edgeReader, Title: "夹", At: at})
	for i, articleID := range []uint64{8_401, 8_402} {
		schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
			ID:              uint64(9_401 + i),
			ArticleID:       articleID,
			UserID:          edgeReader,
			FavorID:         401,
			ArticleTitle:    "文章",
			ArticleStatus:   schema.ArticleStatusPublished,
			ArticleAuthorID: edgeReader,
			At:              at,
		})
	}

	repo := mysql.NewFavoriteRepo(db)
	ctx := context.Background()

	items, total, err := repo.ListItems(ctx, ctype.NewID(401), 0, 10, false, false)
	if err != nil {
		t.Fatalf("列收藏内容（本人）失败: %v", err)
	}
	if len(items) != 2 || total != 2 {
		t.Fatalf("本人应看到 2 条，实际 list=%d total=%d", len(items), total)
	}

	items, total, err = repo.ListItems(ctx, ctype.NewID(401), 0, 10, false, true)
	if err != nil {
		t.Fatalf("列收藏内容（公开面）失败: %v", err)
	}
	if len(items) != 1 || total != 1 {
		t.Fatalf("公开面只应看到 1 条已发布，实际 list=%d total=%d", len(items), total)
	}
	if items[0].ArticleID() != domain.ArticleID(ctype.NewID(8_401)) {
		t.Fatalf("公开面应只剩已发布那篇，实际 %s", items[0].ArticleID())
	}
}
