package cron_service_test

import (
	"context"
	"testing"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/cron_service"
	"myblogx/test/testutil"
)

func TestSyncTagApplyIncrements(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t, &models.TagModel{})

	tag := models.TagModel{Title: "Go"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	if err := setupTagCounter(tag.ID, 5); err != nil {
		t.Fatalf("设置标签缓存失败: %v", err)
	}

	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncTag()

	var got struct {
		ArticleCount int
	}
	if err := db.Model(&models.TagModel{}).
		Select("article_count").
		Where("id = ?", tag.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询标签失败: %v", err)
	}
	if got.ArticleCount != 5 {
		t.Fatalf("同步结果异常: article_count=%d", got.ArticleCount)
	}
}

func TestSyncTagNotBelowZeroAndLockSkip(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t, &models.TagModel{})

	tag := models.TagModel{Title: "Java", ArticleCount: 1}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	if err := setupTagCounter(tag.ID, -5); err != nil {
		t.Fatalf("设置标签缓存失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncTag()

	var got struct {
		ArticleCount int
	}
	if err := db.Model(&models.TagModel{}).
		Select("article_count").
		Where("id = ?", tag.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询标签失败: %v", err)
	}
	if got.ArticleCount != 0 {
		t.Fatalf("标签计数不应小于 0: %d", got.ArticleCount)
	}

	if err := setupTagCounter(tag.ID, 9); err != nil {
		t.Fatalf("设置标签缓存失败: %v", err)
	}
	if err := testutil.Redis().Set(context.Background(), "cron:sync_tag:lock", "manual-lock", 0).Err(); err != nil {
		t.Fatalf("设置锁失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncTag()

	if err := db.Model(&models.TagModel{}).
		Select("article_count").
		Where("id = ?", tag.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询标签失败: %v", err)
	}
	if got.ArticleCount != 0 {
		t.Fatalf("加锁后不应更新 article_count: %d", got.ArticleCount)
	}
}

func setupTagCounter(tagID ctype.ID, delta int) error {
	return testutil.Redis().HIncrBy(context.Background(), "tag_article_count", tagID.String(), int64(delta)).Err()
}
