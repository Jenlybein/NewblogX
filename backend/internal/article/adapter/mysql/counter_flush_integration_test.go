//go:build integration

package mysql_test

import (
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/testkit/schema"
)

// 本文件是「并账按关系行重算覆盖」在**真 MySQL** 上的契约测试（ADR-0016 决策 1）。
//
// 为什么必须真库：写库这条路径有两处只有 MySQL 才会暴露的差异 ——
//   - `affected rows` 是"真正被改动的行数"（新值等于旧值时返回 0），
//     拿它判断"文章是否存在"在 sqlite 上会过、在 MySQL 上会误报；
//   - 取消点赞 / 移出收藏夹是**软删**，残行仍占着唯一键，COUNT 必须带
//     `deleted_at IS NULL`，否则数字永远降不下来。
//
// 用例只做数据准备 + 调一个导出函数，不构造 HTTP / 装配层。

const (
	reconcileAuthor uint64 = 100
	reconcileReader uint64 = 200
	// reconcileCancelled 是"点过赞又取消"的那个用户（软删残行的所有者）。
	reconcileCancelled uint64 = 300
	reconcileArtID     uint64 = 1001
	reconcileFolID     uint64 = 1
)

// TestReconcileArticleCountRecomputesOnMySQL 库内列漂移后一次并账即自愈。
//
// 场景：库内列写成 9（历史上丢过增量），关系行只有 2 赞 1 收藏。
// 并账后两列必须等于关系行条数，而不是 9 加增量 —— 这就是"派生存列"的口径。
func TestReconcileArticleCountRecomputesOnMySQL(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: reconcileArtID, Title: "并账契约文章", AuthorID: reconcileAuthor,
		DiggCount: 9, FavorCount: 9,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
	})
	schema.InsertFavorite(t, db, schema.FavoriteSpec{
		ID: reconcileFolID, UserID: reconcileReader, Title: "默认收藏夹", IsDefault: true,
	})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6001, ArticleID: reconcileArtID, UserID: reconcileReader})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6002, ArticleID: reconcileArtID, UserID: reconcileAuthor})
	// 取消过一次赞：软删残行，不再是"一条关系"。
	// 这一行必须属于**第三个用户**（同一用户对同一篇只能有一条点赞关系），而
	// `article_digg_models.user_id` 有外键 —— 用户行得先存在。这是第一次真跑 MySQL
	// 才暴露的：`seedUsers` 只建了 100 / 200，写死 300 会直接 FK 失败。
	seedUser(t, db, reconcileCancelled)
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6003, ArticleID: reconcileArtID, UserID: reconcileCancelled, SoftDeleted: true})
	schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
		ID: 7001, ArticleID: reconcileArtID, UserID: reconcileReader, FavorID: reconcileFolID, ArticleTitle: "并账契约文章",
	})

	if hit, err := mysql.ReconcileArticleCount(db, mysql.ArticleDiggCount, reconcileArtID, 5); err != nil || !hit {
		t.Fatalf("并账点赞数应命中: hit=%v err=%v", hit, err)
	}
	if hit, err := mysql.ReconcileArticleCount(db, mysql.ArticleFavorCount, reconcileArtID, -100); err != nil || !hit {
		t.Fatalf("并账收藏数应命中: hit=%v err=%v", hit, err)
	}

	got := readArticleCounters(t, db)
	if got.DiggCount != 2 {
		t.Fatalf("点赞数应按关系行重算为 2（增量 5 与漂移都要被覆盖掉），实际 %d", got.DiggCount)
	}
	if got.FavorCount != 1 {
		t.Fatalf("收藏数应按关系行重算为 1（增量 -100 必须被忽略），实际 %d", got.FavorCount)
	}
}

// TestReconcileArticleCountSelfHealsLostDelta 丢过一次增量后，下一个并账周期自愈。
//
// 这是 #252 的核心验收点：关系行 +1 但增量丢了（Redis 中过、进程崩过），
// 库内列停在旧值。并账不再累加而是重算，所以这一轮的列值就回到了关系行条数。
func TestReconcileArticleCountSelfHealsLostDelta(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: reconcileArtID, Title: "并账契约文章", AuthorID: reconcileAuthor,
		DiggCount:     0, // 增量丢了：关系行有 1 条，列还是 0
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
	})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6001, ArticleID: reconcileArtID, UserID: reconcileReader})

	// 本轮没有增量可并（Redis 里那条已经丢了），但并账依然要纠正列。
	if hit, err := mysql.ReconcileArticleCount(db, mysql.ArticleDiggCount, reconcileArtID, 0); err != nil || !hit {
		t.Fatalf("并账应命中: hit=%v err=%v", hit, err)
	}
	if got := readArticleCounters(t, db).DiggCount; got != 1 {
		t.Fatalf("丢增量的漂移应被下一轮并账纠正为 1，实际 %d", got)
	}

	// 再次并账：结果稳定（覆盖写幂等，不会因为跑第二遍变成 2）。
	if _, err := mysql.ReconcileArticleCount(db, mysql.ArticleDiggCount, reconcileArtID, 0); err != nil {
		t.Fatalf("重复并账失败: %v", err)
	}
	if got := readArticleCounters(t, db).DiggCount; got != 1 {
		t.Fatalf("重复并账不应累积，实际 %d", got)
	}
}

// TestReconcileArticleCountLeavesDeltaColumnsAlone view / comment 仍按增量累加。
func TestReconcileArticleCountLeavesDeltaColumnsAlone(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: reconcileArtID, Title: "并账契约文章", AuthorID: reconcileAuthor,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
	})

	if _, err := mysql.ReconcileArticleCount(db, mysql.ArticleViewCount, reconcileArtID, 7); err != nil {
		t.Fatalf("并账浏览数失败: %v", err)
	}
	if _, err := mysql.ReconcileArticleCount(db, mysql.ArticleCommentCount, reconcileArtID, 4); err != nil {
		t.Fatalf("并账评论数失败: %v", err)
	}
	got := readArticleCounters(t, db)
	if got.ViewCount != 7 || got.CommentCount != 4 {
		t.Fatalf("浏览 / 评论仍应按增量落库: view=%d comment=%d", got.ViewCount, got.CommentCount)
	}
}

// readArticleCounters 回读文章行的四个计数列。
func readArticleCounters(t *testing.T, db *gorm.DB) schema.ArticleRow {
	t.Helper()
	var row schema.ArticleRow
	if err := db.First(&row, "id = ?", reconcileArtID).Error; err != nil {
		t.Fatalf("回查文章失败: %v", err)
	}
	return row
}
