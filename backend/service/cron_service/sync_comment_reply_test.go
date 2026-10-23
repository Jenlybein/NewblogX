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

func TestSyncCommentReplyApplyIncrements(t *testing.T) {
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
	root := models.CommentModel{Content: "root", UserID: user.ID, ArticleID: article.ID}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}

	if err := setupReplyCounter(root.ID, 5); err != nil {
		t.Fatalf("设置reply缓存失败: %v", err)
	}

	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentReply()

	var got struct {
		ReplyCount int
	}
	if err := db.Model(&models.CommentModel{}).
		Select("reply_count").
		Where("id = ?", root.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.ReplyCount != 5 {
		t.Fatalf("同步reply_count异常: %d", got.ReplyCount)
	}
}

func TestSyncCommentReplyNotBelowZeroAndLockSkip(t *testing.T) {
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
	root := models.CommentModel{Content: "root", UserID: user.ID, ArticleID: article.ID, ReplyCount: 1}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}

	if err := setupReplyCounter(root.ID, -5); err != nil {
		t.Fatalf("设置reply缓存失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentReply()

	var got struct {
		ReplyCount int
	}
	if err := db.Model(&models.CommentModel{}).
		Select("reply_count").
		Where("id = ?", root.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.ReplyCount != 0 {
		t.Fatalf("reply_count不应小于0: %d", got.ReplyCount)
	}

	if err := setupReplyCounter(root.ID, 9); err != nil {
		t.Fatalf("设置reply缓存失败: %v", err)
	}
	if err := setKey("cron:sync_comment_reply:lock", "manual-lock"); err != nil {
		t.Fatalf("设置锁失败: %v", err)
	}
	cron_service.NewSchedulerRaw(db, testutil.Redis(), testutil.Logger()).SyncCommentReply()

	if err := db.Model(&models.CommentModel{}).
		Select("reply_count").
		Where("id = ?", root.ID).
		Scan(&got).Error; err != nil {
		t.Fatalf("查询评论失败: %v", err)
	}
	if got.ReplyCount != 0 {
		t.Fatalf("加锁后不应更新reply_count: %d", got.ReplyCount)
	}
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	if redis_comment.GetCacheReply(deps, root.ID) != 9 {
		t.Fatalf("加锁后缓存应保留: %d", redis_comment.GetCacheReply(deps, root.ID))
	}
}

func setupReplyCounter(commentID ctype.ID, delta int) error {
	return testutil.Redis().HIncrBy(context.Background(), redis_comment.ReplyCountCacheKey, commentID.String(), int64(delta)).Err()
}

func setKey(key, value string) error {
	return testutil.Redis().Set(context.Background(), key, value, 0).Err()
}
