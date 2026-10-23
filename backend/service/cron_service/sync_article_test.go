package cron_service_test

import (
	"context"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/cron_service"
	"myblogx/test/testutil"
	"testing"
)

func TestSyncArticleApplyIncrements(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t, &models.ArticleModel{})

	article := models.ArticleModel{
		Title:    "a1",
		AuthorID: 1,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	if err := testutilSetupCounter("article_view", article.ID, 5); err != nil {
		t.Fatalf("设置 view 缓存失败: %v", err)
	}
	if err := testutilSetupCounter("article_digg", article.ID, 2); err != nil {
		t.Fatalf("设置 digg 缓存失败: %v", err)
	}
	if err := testutilSetupCounter("article_favorite", article.ID, 3); err != nil {
		t.Fatalf("设置 favorite 缓存失败: %v", err)
	}
	if err := testutilSetupCounter("article_comment", article.ID, 4); err != nil {
		t.Fatalf("设置 comment 缓存失败: %v", err)
	}

	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncArticle()

	var got struct {
		ViewCount    int
		DiggCount    int
		FavorCount   int
		CommentCount int
	}
	if err := db.Model(&models.ArticleModel{}).
		Select("view_count", "digg_count", "favor_count", "comment_count").
		Where("id = ?", article.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询文章失败: %v", err)
	}
	if got.ViewCount != 5 || got.DiggCount != 2 || got.FavorCount != 3 || got.CommentCount != 4 {
		t.Fatalf("同步结果异常: view=%d digg=%d favor=%d comment=%d", got.ViewCount, got.DiggCount, got.FavorCount, got.CommentCount)
	}
}

func TestSyncArticleNotBelowZeroAndLockSkip(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t, &models.ArticleModel{})

	article := models.ArticleModel{
		Title:        "a2",
		AuthorID:     1,
		DiggCount:    1,
		FavorCount:   1,
		ViewCount:    1,
		CommentCount: 1,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	if err := testutilSetupCounter("article_digg", article.ID, -5); err != nil {
		t.Fatalf("设置 digg 缓存失败: %v", err)
	}
	if err := testutilSetupCounter("article_favorite", article.ID, -5); err != nil {
		t.Fatalf("设置 favorite 缓存失败: %v", err)
	}
	if err := testutilSetupCounter("article_comment", article.ID, -5); err != nil {
		t.Fatalf("设置 comment 缓存失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncArticle()

	var got struct {
		ViewCount    int
		DiggCount    int
		FavorCount   int
		CommentCount int
	}
	if err := db.Model(&models.ArticleModel{}).
		Select("view_count", "digg_count", "favor_count", "comment_count").
		Where("id = ?", article.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询文章失败: %v", err)
	}
	if got.DiggCount != 0 || got.FavorCount != 0 || got.CommentCount != 0 {
		t.Fatalf("计数不应小于0: digg=%d favor=%d comment=%d", got.DiggCount, got.FavorCount, got.CommentCount)
	}

	// 加锁后本轮应跳过
	if err := testutilSetupCounter("article_view", article.ID, 9); err != nil {
		t.Fatalf("设置 view 缓存失败: %v", err)
	}
	if err := testutilSetKey("cron:sync_article:lock", "manual-lock"); err != nil {
		t.Fatalf("设置锁失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncArticle()

	if err := db.Model(&models.ArticleModel{}).
		Select("view_count", "digg_count", "favor_count", "comment_count").
		Where("id = ?", article.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询文章失败: %v", err)
	}
	if got.ViewCount != 1 {
		t.Fatalf("加锁后不应更新 view: %d", got.ViewCount)
	}
}

func testutilSetupCounter(key string, articleID ctype.ID, delta int) error {
	return testutil.Redis().HIncrBy(context.Background(), key, articleID.String(), int64(delta)).Err()
}

func testutilSetKey(key, value string) error {
	return testutil.Redis().Set(context.Background(), key, value, 0).Err()
}
