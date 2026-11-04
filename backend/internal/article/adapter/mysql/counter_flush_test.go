package mysql

import (
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// 本文件是包内测试：直接用 row 结构建表，不经过 models，
// 与 adapter 内部使用的是同一份表定义。
//
// 并账有两种写库方式（ADR-0025 决策 1）：
//   - `digg_count` / `favor_count`：按关系行 `COUNT` **重算覆盖**，不看增量；
//   - `view_count` / `comment_count`：没有等价关系行，仍是 `列 += 增量`。

// TestReconcileArticleCountRecomputesSocialColumns 并账把两个社列重算成关系行条数。
//
// 库内列故意写成 9（漂移），Redis 增量再给一个夸张的值：结果必须只由关系行决定。
// 这条用例就是「不再累加」的判据 —— 任何把 `列 += 增量` 加回来的实现都会红。
func TestReconcileArticleCountRecomputesSocialColumns(t *testing.T) {
	db := testutil.SetupSQLite(t, &articleRow{}, &articleDiggRow{}, &userArticleFavorRow{})

	row := articleRow{Title: "drifted", DiggCount: 9, FavorCount: 9}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	id := row.ID.Uint64()

	// 真值：2 个赞、1 条收藏关系。
	seedArticleDiggRow(t, db, row.ID, 301)
	seedArticleDiggRow(t, db, row.ID, 302)
	if err := db.Create(&userArticleFavorRow{
		ArticleID: row.ID, UserID: ctype.NewID(401), FavorID: ctype.NewID(1), ArticleTitle: "被收藏",
	}).Error; err != nil {
		t.Fatalf("创建收藏关系失败: %v", err)
	}

	if hit, err := ReconcileArticleCount(db, ArticleDiggCount, id, 5); err != nil || !hit {
		t.Fatalf("并账点赞数应命中: hit=%v err=%v", hit, err)
	}
	if hit, err := ReconcileArticleCount(db, ArticleFavorCount, id, -100); err != nil || !hit {
		t.Fatalf("并账收藏数应命中: hit=%v err=%v", hit, err)
	}

	got := mustArticleRow(t, db, row.ID)
	if got.DiggCount != 2 {
		t.Fatalf("点赞数应按关系行重算为 2（增量 5 必须被忽略），实际 %d", got.DiggCount)
	}
	if got.FavorCount != 1 {
		t.Fatalf("收藏数应按关系行重算为 1（增量 -100 必须被忽略），实际 %d", got.FavorCount)
	}
}

// TestReconcileArticleCountIgnoresSoftDeletedRelations 软删的关系行不算收藏 / 点赞。
//
// 取消点赞与移出收藏夹都是软删（唯一键不含 deleted_at，行留在表里），
// 所以「关系行条数」必须是带 `deleted_at IS NULL` 的条数，否则取消点赞后数字不降。
func TestReconcileArticleCountIgnoresSoftDeletedRelations(t *testing.T) {
	db := testutil.SetupSQLite(t, &articleRow{}, &articleDiggRow{}, &userArticleFavorRow{})

	row := articleRow{Title: "a1"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	kept := seedArticleDiggRow(t, db, row.ID, 301)
	removed := seedArticleDiggRow(t, db, row.ID, 302)
	if err := db.Delete(&articleDiggRow{}, "id = ?", removed.ID).Error; err != nil {
		t.Fatalf("软删点赞行失败: %v", err)
	}
	if kept.ID.IsZero() {
		t.Fatal("预置点赞行失败")
	}

	if _, err := ReconcileArticleCount(db, ArticleDiggCount, row.ID.Uint64(), 0); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	if got := mustArticleRow(t, db, row.ID).DiggCount; got != 1 {
		t.Fatalf("软删的点赞行不应计入，点赞数应为 1，实际 %d", got)
	}
}

// TestReconcileArticleCountKeepsDeltaColumns view / comment 仍按增量累加。
//
// 这两列没有等价关系行（浏览量没有关系表；评论数的口径由评论模块自己维护），
// 所以本批只改两个社列，其余列的既有行为不能被顺手改掉。
func TestReconcileArticleCountKeepsDeltaColumns(t *testing.T) {
	db := testutil.SetupSQLite(t, &articleRow{}, &articleDiggRow{}, &userArticleFavorRow{})

	row := articleRow{Title: "a1"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	id := row.ID.Uint64()

	for _, c := range []struct {
		col   ArticleCounter
		delta int
	}{
		{ArticleViewCount, 5},
		{ArticleCommentCount, 4},
	} {
		if hit, err := ReconcileArticleCount(db, c.col, id, c.delta); err != nil || !hit {
			t.Fatalf("%s 并账应命中: hit=%v err=%v", c.col, hit, err)
		}
	}

	got := mustArticleRow(t, db, row.ID)
	if got.ViewCount != 5 || got.CommentCount != 4 {
		t.Fatalf("浏览 / 评论仍应按增量落库: view=%d comment=%d", got.ViewCount, got.CommentCount)
	}
}

// TestReconcileArticleCountFloorAtZero 负数增量不会把计数压到 0 以下。
// 页面上出现"文章数 -2"比少 1 更难解释。
func TestReconcileArticleCountFloorAtZero(t *testing.T) {
	db := testutil.SetupSQLite(t, &articleRow{}, &articleDiggRow{}, &userArticleFavorRow{})

	row := articleRow{Title: "a1"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	id := row.ID.Uint64()

	if _, err := ReconcileArticleCount(db, ArticleViewCount, id, 1); err != nil {
		t.Fatalf("加 1 失败: %v", err)
	}
	if _, err := ReconcileArticleCount(db, ArticleViewCount, id, -5); err != nil {
		t.Fatalf("减 5 失败: %v", err)
	}
	if got := mustArticleRow(t, db, row.ID).ViewCount; got != 0 {
		t.Fatalf("计数应停在 0，实际=%d", got)
	}
}

// TestReconcileArticleCountMissingRow 文章不存在时返回 hit=false 而不是错误。
//
// 这个区分很关键：调用方靠它决定"丢弃增量"还是"回补 Redis 下轮重试"。
// 如果这里返回 error，一条已删除的文章会让任务无限重试同一行。
func TestReconcileArticleCountMissingRow(t *testing.T) {
	db := testutil.SetupSQLite(t, &articleRow{}, &articleDiggRow{}, &userArticleFavorRow{})

	for _, col := range []ArticleCounter{ArticleViewCount, ArticleDiggCount, ArticleFavorCount} {
		hit, err := ReconcileArticleCount(db, col, 999999, 1)
		if err != nil {
			t.Fatalf("%s: 文章不存在不应返回 error，实际=%v", col, err)
		}
		if hit {
			t.Fatalf("%s: 文章不存在时应返回 hit=false", col)
		}
	}
}

func TestFlushTagArticleCount(t *testing.T) {
	db := testutil.SetupSQLite(t, &tagRow{})

	row := tagRow{Title: "go"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	id := row.ID.Uint64()

	if hit, err := FlushTagArticleCount(db, id, 3); err != nil || !hit {
		t.Fatalf("加 3 应命中: hit=%v err=%v", hit, err)
	}
	if hit, err := FlushTagArticleCount(db, id, -10); err != nil || !hit {
		t.Fatalf("减 10 应命中: hit=%v err=%v", hit, err)
	}

	var got tagRow
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.ArticleCount != 0 {
		t.Fatalf("标签文章数应停在 0，实际=%d", got.ArticleCount)
	}

	if hit, err := FlushTagArticleCount(db, 999999, 1); err != nil || hit {
		t.Fatalf("标签不存在应返回 hit=false 且无 error: hit=%v err=%v", hit, err)
	}
}

// ── 辅助 ──

func seedArticleDiggRow(t *testing.T, db *gorm.DB, articleID ctype.ID, userID uint64) *articleDiggRow {
	t.Helper()
	row := &articleDiggRow{ArticleID: articleID, UserID: ctype.NewID(userID)}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("创建点赞关系失败: %v", err)
	}
	return row
}

func mustArticleRow(t *testing.T, db *gorm.DB, id ctype.ID) articleRow {
	t.Helper()
	var got articleRow
	if err := db.First(&got, id).Error; err != nil {
		t.Fatalf("回查文章失败: %v", err)
	}
	return got
}
