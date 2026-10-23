package comment_api

import (
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"testing"
)

func TestCommentDiggView(t *testing.T) {
	user := setupCommentEnv(t)
	api := setupCommentAPI(t)

	article := models.ArticleModel{
		Title:          "digg-article",
		Content:        "c",
		AuthorID:       user.ID,
		CommentsToggle: true,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	published := models.CommentModel{
		Content:   "published",
		UserID:    user.ID,
		ArticleID: article.ID,
		Status:    enum.CommentStatusPublished,
	}
	if err := testutil.DB().Create(&published).Error; err != nil {
		t.Fatalf("创建已发布评论失败: %v", err)
	}

	examining := models.CommentModel{
		Content:   "examining",
		UserID:    user.ID,
		ArticleID: article.ID,
		Status:    enum.CommentStatusExamining,
	}
	if err := testutil.DB().Create(&examining).Error; err != nil {
		t.Fatalf("创建审核中评论失败: %v", err)
	}

	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser, Username: user.Username}}

	t.Run("点赞与取消点赞", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", claims)
		c.Set("requestUri", models.IDRequest{ID: published.ID})
		api.CommentDiggView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("点赞应成功 body=%s", w.Body.String())
		}

		var cnt int64
		if err := testutil.DB().Model(&models.CommentDiggModel{}).
			Where("comment_id = ? and user_id = ?", published.ID, user.ID).
			Count(&cnt).Error; err != nil {
			t.Fatalf("查询点赞记录失败: %v", err)
		}
		if cnt != 1 {
			t.Fatalf("点赞记录数量错误: %d", cnt)
		}
		if redis_comment.GetCacheDigg(testRedisDeps(), published.ID) != 1 {
			t.Fatalf("点赞缓存计数错误: %d", redis_comment.GetCacheDigg(testRedisDeps(), published.ID))
		}

		c2, w2 := newCommentCtx()
		c2.Set("claims", claims)
		c2.Set("requestUri", models.IDRequest{ID: published.ID})
		api.CommentDiggView(c2)
		if code := readBizCode(t, w2); code != 0 {
			t.Fatalf("取消点赞应成功 body=%s", w2.Body.String())
		}
		if err := testutil.DB().Model(&models.CommentDiggModel{}).
			Where("comment_id = ? and user_id = ?", published.ID, user.ID).
			Count(&cnt).Error; err != nil {
			t.Fatalf("查询点赞记录失败: %v", err)
		}
		if cnt != 0 {
			t.Fatalf("点赞记录应删除: %d", cnt)
		}
		if redis_comment.GetCacheDigg(testRedisDeps(), published.ID) != 0 {
			t.Fatalf("取消点赞后缓存应归零: %d", redis_comment.GetCacheDigg(testRedisDeps(), published.ID))
		}

		c3, w3 := newCommentCtx()
		c3.Set("claims", claims)
		c3.Set("requestUri", models.IDRequest{ID: published.ID})
		api.CommentDiggView(c3)
		if code := readBizCode(t, w3); code != 0 {
			t.Fatalf("软删后重新点赞应成功 body=%s", w3.Body.String())
		}
		if err := testutil.DB().Unscoped().Model(&models.CommentDiggModel{}).
			Where("comment_id = ? and user_id = ?", published.ID, user.ID).
			Count(&cnt).Error; err != nil {
			t.Fatalf("查询点赞记录失败: %v", err)
		}
		if cnt != 1 {
			t.Fatalf("软删恢复后应复用同一条点赞记录: %d", cnt)
		}
	})

	t.Run("审核中评论不允许点赞", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", claims)
		c.Set("requestUri", models.IDRequest{ID: examining.ID})
		api.CommentDiggView(c)
		if code := readBizCode(t, w); code == 0 {
			t.Fatalf("审核中评论点赞应失败 body=%s", w.Body.String())
		}
	})
}
