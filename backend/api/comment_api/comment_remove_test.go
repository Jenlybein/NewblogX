package comment_api

import (
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"testing"
)

func TestCommentRemoveView(t *testing.T) {
	owner := setupCommentEnv(t)
	api := setupCommentAPI(t)

	commenter := &models.UserModel{Username: "commenter", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(commenter).Error; err != nil {
		t.Fatalf("创建 commenter 失败: %v", err)
	}
	other := &models.UserModel{Username: "other", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(other).Error; err != nil {
		t.Fatalf("创建 other 失败: %v", err)
	}
	admin := &models.UserModel{Username: "admin", Password: "x", Role: enum.RoleAdmin}
	if err := testutil.DB().Create(admin).Error; err != nil {
		t.Fatalf("创建 admin 失败: %v", err)
	}

	articleOwner := models.ArticleModel{Title: "owner-article", Content: "c", AuthorID: owner.ID, CommentsToggle: true}
	if err := testutil.DB().Create(&articleOwner).Error; err != nil {
		t.Fatalf("创建 owner 文章失败: %v", err)
	}
	articleOther := models.ArticleModel{Title: "other-article", Content: "c", AuthorID: other.ID, CommentsToggle: true}
	if err := testutil.DB().Create(&articleOther).Error; err != nil {
		t.Fatalf("创建 other 文章失败: %v", err)
	}

	t.Run("文章作者删除根评论时级联删除二级评论", func(t *testing.T) {
		root := models.CommentModel{
			Content:   "root-delete",
			UserID:    commenter.ID,
			ArticleID: articleOwner.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&root).Error; err != nil {
			t.Fatalf("创建根评论失败: %v", err)
		}
		reply := models.CommentModel{
			Content:   "reply-delete",
			UserID:    other.ID,
			ArticleID: articleOwner.ID,
			ReplyId:   root.ID,
			RootID:    root.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&reply).Error; err != nil {
			t.Fatalf("创建二级评论失败: %v", err)
		}
		pendingReply := models.CommentModel{
			Content:   "reply-pending-delete",
			UserID:    other.ID,
			ArticleID: articleOwner.ID,
			ReplyId:   root.ID,
			RootID:    root.ID,
			Status:    enum.CommentStatusExamining,
		}
		if err := testutil.DB().Create(&pendingReply).Error; err != nil {
			t.Fatalf("创建待审核二级评论失败: %v", err)
		}

		if err := redis_article.SetCacheComment(testRedisDeps(), articleOwner.ID, 5); err != nil {
			t.Fatalf("写入文章评论缓存失败: %v", err)
		}

		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: owner.ID, Role: enum.RoleUser, Username: owner.Username}})
		c.Set("requestUri", models.IDRequest{ID: root.ID})
		api.CommentRemoveView(c)

		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("文章作者删除评论应成功 body=%s", w.Body.String())
		}

		var count int64
		if err := testutil.DB().Model(&models.CommentModel{}).
			Where("id IN ?", []ctype.ID{root.ID, reply.ID, pendingReply.ID}).
			Count(&count).Error; err != nil {
			t.Fatalf("查询删除结果失败: %v", err)
		}
		if count != 0 {
			t.Fatalf("根评论删除应级联删除二级评论, count=%d", count)
		}

		if redis_article.GetCacheComment(testRedisDeps(), articleOwner.ID) != 3 {
			t.Fatalf("文章评论缓存未按已发布评论回滚: %d", redis_article.GetCacheComment(testRedisDeps(), articleOwner.ID))
		}
	})

	t.Run("用户可以删除自己发布的评论", func(t *testing.T) {
		selfComment := models.CommentModel{
			Content:   "self-delete",
			UserID:    commenter.ID,
			ArticleID: articleOther.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&selfComment).Error; err != nil {
			t.Fatalf("创建自评论失败: %v", err)
		}
		if err := redis_article.SetCacheComment(testRedisDeps(), articleOther.ID, 2); err != nil {
			t.Fatalf("写入文章评论缓存失败: %v", err)
		}

		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: commenter.ID, Role: enum.RoleUser, Username: commenter.Username}})
		c.Set("requestUri", models.IDRequest{ID: selfComment.ID})
		api.CommentRemoveView(c)

		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("用户删除自己的评论应成功 body=%s", w.Body.String())
		}

		var count int64
		if err := testutil.DB().Model(&models.CommentModel{}).Where("id = ?", selfComment.ID).Count(&count).Error; err != nil {
			t.Fatalf("查询删除结果失败: %v", err)
		}
		if count != 0 {
			t.Fatalf("用户删除自己评论失败, count=%d", count)
		}
		if redis_article.GetCacheComment(testRedisDeps(), articleOther.ID) != 1 {
			t.Fatalf("文章评论缓存未正确回滚: %d", redis_article.GetCacheComment(testRedisDeps(), articleOther.ID))
		}
	})

	t.Run("删除已发布二级评论会回滚根评论Reply缓存", func(t *testing.T) {
		root := models.CommentModel{
			Content:   "root-for-reply-cache",
			UserID:    other.ID,
			ArticleID: articleOther.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&root).Error; err != nil {
			t.Fatalf("创建根评论失败: %v", err)
		}
		reply := models.CommentModel{
			Content:   "reply-for-reply-cache",
			UserID:    commenter.ID,
			ArticleID: articleOther.ID,
			ReplyId:   root.ID,
			RootID:    root.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&reply).Error; err != nil {
			t.Fatalf("创建二级评论失败: %v", err)
		}
		if err := redis_comment.SetCacheReply(testRedisDeps(), root.ID, 3); err != nil {
			t.Fatalf("写入根评论reply缓存失败: %v", err)
		}

		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: commenter.ID, Role: enum.RoleUser, Username: commenter.Username}})
		c.Set("requestUri", models.IDRequest{ID: reply.ID})
		api.CommentRemoveView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("删除二级评论应成功 body=%s", w.Body.String())
		}

		var count int64
		if err := testutil.DB().Model(&models.CommentModel{}).Where("id = ?", reply.ID).Count(&count).Error; err != nil {
			t.Fatalf("查询删除结果失败: %v", err)
		}
		if count != 0 {
			t.Fatalf("二级评论应被删除, count=%d", count)
		}
		if redis_comment.GetCacheReply(testRedisDeps(), root.ID) != 2 {
			t.Fatalf("根评论reply缓存未回滚, got=%d", redis_comment.GetCacheReply(testRedisDeps(), root.ID))
		}
	})

	t.Run("管理员可以删除任意评论", func(t *testing.T) {
		target := models.CommentModel{
			Content:   "admin-delete",
			UserID:    other.ID,
			ArticleID: articleOwner.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&target).Error; err != nil {
			t.Fatalf("创建管理员删除目标失败: %v", err)
		}
		before := redis_article.GetCacheComment(testRedisDeps(), articleOwner.ID)

		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Role: enum.RoleAdmin, Username: admin.Username}})
		c.Set("requestUri", models.IDRequest{ID: target.ID})
		api.CommentRemoveView(c)

		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("管理员删除评论应成功 body=%s", w.Body.String())
		}

		var count int64
		if err := testutil.DB().Model(&models.CommentModel{}).Where("id = ?", target.ID).Count(&count).Error; err != nil {
			t.Fatalf("查询删除结果失败: %v", err)
		}
		if count != 0 {
			t.Fatalf("管理员删除评论失败, count=%d", count)
		}
		if redis_article.GetCacheComment(testRedisDeps(), articleOwner.ID) != before-1 {
			t.Fatalf("管理员删除后文章评论缓存未正确回滚: before=%d after=%d", before, redis_article.GetCacheComment(testRedisDeps(), articleOwner.ID))
		}
	})

	t.Run("无权限用户不能删除他人评论", func(t *testing.T) {
		target := models.CommentModel{
			Content:   "forbidden-delete",
			UserID:    other.ID,
			ArticleID: articleOther.ID,
			Status:    enum.CommentStatusPublished,
		}
		if err := testutil.DB().Create(&target).Error; err != nil {
			t.Fatalf("创建无权限删除目标失败: %v", err)
		}

		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: commenter.ID, Role: enum.RoleUser, Username: commenter.Username}})
		c.Set("requestUri", models.IDRequest{ID: target.ID})
		api.CommentRemoveView(c)

		if code := readBizCode(t, w); code == 0 {
			t.Fatalf("无权限删除应失败 body=%s", w.Body.String())
		}

		var count int64
		if err := testutil.DB().Model(&models.CommentModel{}).Where("id = ?", target.ID).Count(&count).Error; err != nil {
			t.Fatalf("查询删除结果失败: %v", err)
		}
		if count != 1 {
			t.Fatalf("无权限删除不应影响数据, count=%d", count)
		}
	})
}
