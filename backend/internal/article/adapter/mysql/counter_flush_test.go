package mysql

import (
	"testing"

	"myblogx/test/testutil"
)

// 本文件是包内测试：直接用 row 结构建表，不经过 models，
// 与 adapter 内部使用的是同一份表定义。

func TestFlushArticleCountApplies(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &articleRow{})

	row := articleRow{Title: "a1"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	for _, c := range []struct {
		col   ArticleCounter
		delta int
		want  int
	}{
		{ArticleViewCount, 5, 5},
		{ArticleDiggCount, 2, 2},
		{ArticleFavorCount, 3, 3},
		{ArticleCommentCount, 4, 4},
	} {
		if _, err := FlushArticleCount(db, c.col, row.ID.Uint64(), c.delta); err != nil {
			t.Fatalf("%s 落库失败: %v", c.col, err)
		}
	}

	var got articleRow
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.ViewCount != 5 || got.DiggCount != 2 || got.FavorCount != 3 || got.CommentCount != 4 {
		t.Fatalf("落库结果不符: view=%d digg=%d favor=%d comment=%d",
			got.ViewCount, got.DiggCount, got.FavorCount, got.CommentCount)
	}
}

// TestFlushArticleCountFloorAtZero 负数不会把计数压到 0 以下。
// 页面上出现"文章数 -2"比少 1 更难解释。
func TestFlushArticleCountFloorAtZero(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &articleRow{})

	row := articleRow{Title: "a1"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	id := row.ID.Uint64()

	if _, err := FlushArticleCount(db, ArticleDiggCount, id, 1); err != nil {
		t.Fatalf("加 1 失败: %v", err)
	}
	if _, err := FlushArticleCount(db, ArticleDiggCount, id, -5); err != nil {
		t.Fatalf("减 5 失败: %v", err)
	}

	var got articleRow
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.DiggCount != 0 {
		t.Fatalf("计数应停在 0，实际=%d", got.DiggCount)
	}
}

// TestFlushArticleCountMissingRow 文章不存在时返回 hit=false 而不是错误。
//
// 这个区分很关键：调用方靠它决定"丢弃增量"还是"回补 Redis 下轮重试"。
// 如果这里返回 error，一条已删除的文章会让任务无限重试同一行。
func TestFlushArticleCountMissingRow(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &articleRow{})

	hit, err := FlushArticleCount(db, ArticleViewCount, 999999, 1)
	if err != nil {
		t.Fatalf("文章不存在不应返回 error，实际=%v", err)
	}
	if hit {
		t.Fatal("文章不存在时应返回 hit=false")
	}
}

func TestFlushTagArticleCount(t *testing.T) {
	t.Parallel()
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
