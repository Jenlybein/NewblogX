package comment_api

import (
	"encoding/json"
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/models/enum/relationship_enum"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"testing"
)

func TestCommentManListView(t *testing.T) {
	owner := setupCommentEnv(t)
	api := setupCommentAPI(t)

	if err := testutil.DB().Model(owner).Updates(map[string]any{
		"nickname": "owner",
		"avatar":   "/owner.png",
	}).Error; err != nil {
		t.Fatalf("更新 owner 资料失败: %v", err)
	}

	user2 := &models.UserModel{Username: "u2", Nickname: "u2", Avatar: "/u2.png", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(user2).Error; err != nil {
		t.Fatalf("创建 user2 失败: %v", err)
	}
	user3 := &models.UserModel{Username: "u3", Nickname: "u3", Avatar: "/u3.png", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(user3).Error; err != nil {
		t.Fatalf("创建 user3 失败: %v", err)
	}
	admin := &models.UserModel{Username: "admin", Nickname: "admin", Avatar: "/admin.png", Password: "x", Role: enum.RoleAdmin}
	if err := testutil.DB().Create(admin).Error; err != nil {
		t.Fatalf("创建 admin 失败: %v", err)
	}
	followRows := []models.UserFollowModel{
		{FollowedUserID: owner.ID, FansUserID: user2.ID},
		{FollowedUserID: user3.ID, FansUserID: owner.ID},
	}
	if err := testutil.DB().Create(&followRows).Error; err != nil {
		t.Fatalf("创建关注关系失败: %v", err)
	}

	articleMine1 := models.ArticleModel{Title: "mine-1", Content: "c", AuthorID: owner.ID, CommentsToggle: true}
	articleMine2 := models.ArticleModel{Title: "mine-2", Content: "c", AuthorID: owner.ID, CommentsToggle: true}
	articleOther := models.ArticleModel{Title: "other", Content: "c", AuthorID: user2.ID, CommentsToggle: true}
	if err := testutil.DB().Create(&articleMine1).Error; err != nil {
		t.Fatalf("创建 articleMine1 失败: %v", err)
	}
	if err := testutil.DB().Create(&articleMine2).Error; err != nil {
		t.Fatalf("创建 articleMine2 失败: %v", err)
	}
	if err := testutil.DB().Create(&articleOther).Error; err != nil {
		t.Fatalf("创建 articleOther 失败: %v", err)
	}

	minePublished1 := models.CommentModel{Content: "mine_p1", UserID: user2.ID, ArticleID: articleMine1.ID, ReplyCount: 1, Status: enum.CommentStatusPublished}
	minePublished2 := models.CommentModel{Content: "mine_p2", UserID: user3.ID, ArticleID: articleMine2.ID, Status: enum.CommentStatusPublished}
	mineExaminingByOwner := models.CommentModel{Content: "mine_owner_exam", UserID: owner.ID, ArticleID: articleMine1.ID, Status: enum.CommentStatusExamining}
	otherPublished := models.CommentModel{Content: "other_p", UserID: user3.ID, ArticleID: articleOther.ID, Status: enum.CommentStatusPublished}
	ownerOnOther := models.CommentModel{Content: "owner_on_other", UserID: owner.ID, ArticleID: articleOther.ID, Status: enum.CommentStatusPublished}

	for _, item := range []models.CommentModel{minePublished1, minePublished2, mineExaminingByOwner, otherPublished, ownerOnOther} {
		it := item
		if err := testutil.DB().Create(&it).Error; err != nil {
			t.Fatalf("创建评论失败: %v", err)
		}
		if it.Content == "mine_p1" {
			minePublished1.ID = it.ID
		}
	}

	if err := redis_comment.SetCacheReply(testRedisDeps(), minePublished1.ID, 2); err != nil {
		t.Fatalf("写入 reply 缓存失败: %v", err)
	}
	if err := redis_comment.SetCacheDigg(testRedisDeps(), minePublished1.ID, 3); err != nil {
		t.Fatalf("写入 digg 缓存失败: %v", err)
	}

	t.Run("type=1 查询我文章下评论", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: owner.ID, Username: owner.Username, Role: enum.RoleUser}})
		c.Set("requestQuery", CommentManListRequest{
			PageInfo: common.PageInfo{Limit: 20, Page: 1},
			Type:     1,
			Status:   enum.CommentStatusPublished,
		})

		api.CommentManListView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("type=1 查询应成功 body=%s", w.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		data := body["data"].(map[string]any)
		if int(data["count"].(float64)) != 2 {
			t.Fatalf("type=1 count 错误: %+v", data)
		}

		items := data["list"].([]any)
		contentMap := map[string]map[string]any{}
		for _, raw := range items {
			it := raw.(map[string]any)
			contentMap[it["content"].(string)] = it
		}
		if _, ok := contentMap["mine_p1"]; !ok {
			t.Fatalf("缺少 mine_p1: %+v", contentMap)
		}
		if _, ok := contentMap["mine_p2"]; !ok {
			t.Fatalf("缺少 mine_p2: %+v", contentMap)
		}
		if _, ok := contentMap["other_p"]; ok {
			t.Fatalf("不应包含 other_p: %+v", contentMap)
		}
		if int(contentMap["mine_p1"]["reply_count"].(float64)) != 3 {
			t.Fatalf("mine_p1 reply_count 未叠加缓存: %+v", contentMap["mine_p1"])
		}
		if int(contentMap["mine_p1"]["digg_count"].(float64)) != 3 {
			t.Fatalf("mine_p1 digg_count 未叠加缓存: %+v", contentMap["mine_p1"])
		}
		if int(contentMap["mine_p1"]["relation"].(float64)) != int(relationship_enum.RelationFans) {
			t.Fatalf("mine_p1 relation 错误: %+v", contentMap["mine_p1"])
		}
		if int(contentMap["mine_p2"]["relation"].(float64)) != int(relationship_enum.RelationFollowed) {
			t.Fatalf("mine_p2 relation 错误: %+v", contentMap["mine_p2"])
		}
	})

	t.Run("type=2 查询我发的评论", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: owner.ID, Username: owner.Username, Role: enum.RoleUser}})
		c.Set("requestQuery", CommentManListRequest{
			PageInfo: common.PageInfo{Limit: 20, Page: 1},
			Type:     2,
		})

		api.CommentManListView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("type=2 查询应成功 body=%s", w.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		data := body["data"].(map[string]any)
		if int(data["count"].(float64)) != 2 {
			t.Fatalf("type=2 count 错误: %+v", data)
		}

		items := data["list"].([]any)
		contentMap := map[string]bool{}
		for _, raw := range items {
			it := raw.(map[string]any)
			contentMap[it["content"].(string)] = true
		}
		if !contentMap["mine_owner_exam"] || !contentMap["owner_on_other"] {
			t.Fatalf("type=2 返回内容不正确: %+v", contentMap)
		}
	})

	t.Run("type=3 非管理员失败", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user2.ID, Username: user2.Username, Role: enum.RoleUser}})
		c.Set("requestQuery", CommentManListRequest{
			PageInfo: common.PageInfo{Limit: 20, Page: 1},
			Type:     3,
		})

		api.CommentManListView(c)
		if code := readBizCode(t, w); code == 0 {
			t.Fatalf("type=3 非管理员应失败 body=%s", w.Body.String())
		}
	})

	t.Run("type=3 管理员成功并支持 user_id 过滤", func(t *testing.T) {
		c, w := newCommentCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Username: admin.Username, Role: enum.RoleAdmin}})
		c.Set("requestQuery", CommentManListRequest{
			PageInfo: common.PageInfo{Limit: 20, Page: 1},
			Type:     3,
			UserID:   user3.ID,
			Status:   enum.CommentStatusPublished,
		})

		api.CommentManListView(c)
		if code := readBizCode(t, w); code != 0 {
			t.Fatalf("type=3 管理员查询应成功 body=%s", w.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		data := body["data"].(map[string]any)
		if int(data["count"].(float64)) != 2 {
			t.Fatalf("type=3 管理员 count 错误: %+v", data)
		}

		items := data["list"].([]any)
		contentMap := map[string]bool{}
		for _, raw := range items {
			it := raw.(map[string]any)
			contentMap[it["content"].(string)] = true
		}
		if !contentMap["mine_p2"] || !contentMap["other_p"] {
			t.Fatalf("type=3 管理员返回内容不正确: %+v", contentMap)
		}
	})
}
