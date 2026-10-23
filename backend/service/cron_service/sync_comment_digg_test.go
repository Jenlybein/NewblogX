package cron_service_test

import (
	"context"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/cron_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"testing"
)

func TestSyncCommentDiggApplyIncrements(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ArticleModel{},
		&models.CommentModel{},
	)

	user := models.UserModel{Username: "u1", Password: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	article := models.ArticleModel{Title: "a1", Content: "c", AuthorID: user.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	comment := models.CommentModel{Content: "root", UserID: user.ID, ArticleID: article.ID}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}

	if err := setupDiggCounter(comment.ID, 5); err != nil {
		t.Fatalf("设置 digg 缓存失败: %v", err)
	}

	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentDigg()

	var got struct {
		DiggCount int
	}
	if err := db.Model(&models.CommentModel{}).
		Select("digg_count").
		Where("id = ?", comment.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.DiggCount != 5 {
		t.Fatalf("同步 digg_count 异常: %d", got.DiggCount)
	}
}

func TestSyncCommentDiggNotBelowZeroAndLockSkip(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ArticleModel{},
		&models.CommentModel{},
	)

	user := models.UserModel{Username: "u2", Password: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	article := models.ArticleModel{Title: "a2", Content: "c", AuthorID: user.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	comment := models.CommentModel{Content: "root", UserID: user.ID, ArticleID: article.ID, DiggCount: 1}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}

	if err := setupDiggCounter(comment.ID, -5); err != nil {
		t.Fatalf("设置 digg 缓存失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentDigg()

	var got struct {
		DiggCount int
	}
	if err := db.Model(&models.CommentModel{}).
		Select("digg_count").
		Where("id = ?", comment.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.DiggCount != 0 {
		t.Fatalf("digg_count 不应小于0: %d", got.DiggCount)
	}

	if err := setupDiggCounter(comment.ID, 9); err != nil {
		t.Fatalf("设置 digg 缓存失败: %v", err)
	}
	if err := setDiggLockKey("cron:sync_comment_digg:lock", "manual-lock"); err != nil {
		t.Fatalf("设置锁失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentDigg()

	if err := db.Model(&models.CommentModel{}).
		Select("digg_count").
		Where("id = ?", comment.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.DiggCount != 0 {
		t.Fatalf("加锁后不应更新 digg_count: %d", got.DiggCount)
	}
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	if redis_comment.GetCacheDigg(deps, comment.ID) != 9 {
		t.Fatalf("加锁后缓存应保留: %d", redis_comment.GetCacheDigg(deps, comment.ID))
	}
}

func setupDiggCounter(commentID ctype.ID, delta int) error {
	return testutil.Redis().HIncrBy(context.Background(), redis_comment.DiggCountCacheKey, commentID.String(), int64(delta)).Err()
}

func setDiggLockKey(key, value string) error {
	return testutil.Redis().Set(context.Background(), key, value, 0).Err()
}
