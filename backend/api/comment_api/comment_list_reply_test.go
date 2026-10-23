package comment_api

import (
	"encoding/json"
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/models/enum/relationship_enum"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"net/http/httptest"
	"testing"
)

func TestCommentReplyListView(t *testing.T) {
	user := setupCommentEnv(t)
	api := setupCommentAPI(t)

	if err := testutil.DB().Model(user).Updates(map[string]any{
		"nickname": "u1",
		"avatar":   "/u1.png",
	}).Error; err != nil {
		t.Fatalf("更新用户资料失败: %v", err)
	}

	user2 := &models.UserModel{Username: "reply_u", Nickname: "u2", Avatar: "/u2.png", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(user2).Error; err != nil {
		t.Fatalf("创建第二个用户失败: %v", err)
	}
	viewer := &models.UserModel{Username: "reply_viewer", Nickname: "viewer", Avatar: "/v.png", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(viewer).Error; err != nil {
		t.Fatalf("创建访客用户失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserFollowModel{FollowedUserID: viewer.ID, FansUserID: user.ID}).Error; err != nil {
		t.Fatalf("创建 user->viewer 关注关系失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserFollowModel{FollowedUserID: user2.ID, FansUserID: viewer.ID}).Error; err != nil {
		t.Fatalf("创建 viewer->user2 关注关系失败: %v", err)
	}

	article := models.ArticleModel{
		Title:          "open",
		Content:        "c",
		AuthorID:       user.ID,
		CommentsToggle: true,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	root := models.CommentModel{Content: "root", UserID: user.ID, ArticleID: article.ID, ReplyCount: 2, Status: enum.CommentStatusPublished}
	if err := testutil.DB().Create(&root).Error; err != nil {
		t.Fatalf("创建一级评论失败: %v", err)
	}
	reply1 := models.CommentModel{Content: "reply1", UserID: user2.ID, ArticleID: article.ID, ReplyId: root.ID, RootID: root.ID, Status: enum.CommentStatusPublished}
	if err := testutil.DB().Create(&reply1).Error; err != nil {
		t.Fatalf("创建二级评论1失败: %v", err)
	}
	reply2 := models.CommentModel{Content: "reply2", UserID: user.ID, ArticleID: article.ID, ReplyId: reply1.ID, RootID: root.ID, Status: enum.CommentStatusPublished}
	if err := testutil.DB().Create(&reply2).Error; err != nil {
		t.Fatalf("创建二级评论2失败: %v", err)
	}
	replyPending := models.CommentModel{Content: "reply-pending", UserID: user.ID, ArticleID: article.ID, ReplyId: root.ID, RootID: root.ID, Status: enum.CommentStatusExamining}
	if err := testutil.DB().Create(&replyPending).Error; err != nil {
		t.Fatalf("创建待审核二级评论失败: %v", err)
	}

	unpublishedRoot := models.CommentModel{Content: "root-pending", UserID: user.ID, ArticleID: article.ID, Status: enum.CommentStatusExamining}
	if err := testutil.DB().Create(&unpublishedRoot).Error; err != nil {
		t.Fatalf("创建待审核一级评论失败: %v", err)
	}

	if err := redis_comment.SetCacheReply(testRedisDeps(), root.ID, 1); err != nil {
		t.Fatalf("写入 root 回复缓存失败: %v", err)
	}
	if err := redis_comment.SetCacheReply(testRedisDeps(), reply1.ID, 4); err != nil {
		t.Fatalf("写入二级评论回复缓存失败: %v", err)
	}
	if err := redis_comment.SetCacheDigg(testRedisDeps(), reply1.ID, 3); err != nil {
		t.Fatalf("写入二级评论点赞缓存失败: %v", err)
	}
	if err := testutil.DB().Create(&models.CommentDiggModel{CommentID: reply1.ID, UserID: viewer.ID}).Error; err != nil {
		t.Fatalf("创建评论点赞关系失败: %v", err)
	}

	t.Run("分页获取已发布二级评论成功", func(t *testing.T) {
		c, w := newCommentCtx()
		token := testutil.IssueAccessToken(t, viewer)
		req := httptest.NewRequest("GET", "/comments/replies", nil)
		req.Header.Set("token", token)
		c.Request = req
		c.Set("requestQuery", CommentReplyListRequest{
			PageInfo:  common.PageInfo{Limit: 10, Page: 1},
			ArticleID: article.ID,
			RootID:    root.ID,
		})

		api.CommentReplyListView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("获取二级评论应成功，body=%s", w.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		data := body["data"].(map[string]any)
		if data["has_more"].(bool) {
			t.Fatalf("二级评论列表不应还有更多: %+v", data)
		}
		if int(data["reply_count"].(float64)) != 3 {
			t.Fatalf("root reply_count 错误: %+v", data)
		}
		list := data["list"].([]any)
		if len(list) != 2 {
			t.Fatalf("list 长度错误: %d", len(list))
		}

		var reply1Item map[string]any
		for _, raw := range list {
			item := raw.(map[string]any)
			if item["content"] == "reply1" {
				reply1Item = item
				break
			}
		}
		if reply1Item == nil {
			t.Fatalf("未返回 reply1: %+v", list)
		}
		if reply1Item["reply_id"].(string) != root.ID.String() {
			t.Fatalf("reply_id 错误: %+v", reply1Item)
		}
		if int(reply1Item["reply_count"].(float64)) != 4 {
			t.Fatalf("二级评论 reply_count 未叠加缓存: %+v", reply1Item)
		}
		if int(reply1Item["digg_count"].(float64)) != 3 {
			t.Fatalf("二级评论 digg_count 未叠加缓存: %+v", reply1Item)
		}
		if !reply1Item["is_digg"].(bool) {
			t.Fatalf("二级评论 is_digg 应为 true: %+v", reply1Item)
		}
		if int(reply1Item["relation"].(float64)) != int(relationship_enum.RelationFollowed) {
			t.Fatalf("二级评论 relation 异常: %+v", reply1Item)
		}
		if reply1Item["user_id"].(string) != user2.ID.String() {
			t.Fatalf("user_id 应按字符串返回: %+v", reply1Item)
		}

		var reply2Item map[string]any
		for _, raw := range list {
			item := raw.(map[string]any)
			if item["content"] == "reply2" {
				reply2Item = item
				break
			}
		}
		if reply2Item == nil {
			t.Fatalf("未返回 reply2: %+v", list)
		}
		if reply2Item["is_digg"].(bool) {
			t.Fatalf("reply2 is_digg 应为 false: %+v", reply2Item)
		}
		if int(reply2Item["relation"].(float64)) != int(relationship_enum.RelationFans) {
			t.Fatalf("reply2 relation 异常: %+v", reply2Item)
		}
	})

	t.Run("root_id 不是一级评论时失败", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("requestQuery", CommentReplyListRequest{
			PageInfo:  common.PageInfo{Limit: 10, Page: 1},
			ArticleID: article.ID,
			RootID:    reply1.ID,
		})

		api.CommentReplyListView(c)
		if code := readBizCode(t, w); code == 0 {
			t.Fatalf("二级评论作为 root_id 应失败，body=%s", w.Body.String())
		}
	})

	t.Run("root_id 未发布时失败", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("requestQuery", CommentReplyListRequest{
			PageInfo:  common.PageInfo{Limit: 10, Page: 1},
			ArticleID: article.ID,
			RootID:    unpublishedRoot.ID,
		})

		api.CommentReplyListView(c)
		if code := readBizCode(t, w); code == 0 {
			t.Fatalf("未发布一级评论应失败 body=%s", w.Body.String())
		}
	})
}
