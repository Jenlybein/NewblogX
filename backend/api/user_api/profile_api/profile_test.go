package profile_api_test

import (
	"encoding/json"
	"myblogx/api/user_api/profile_api"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("_jwt_config", testutil.Config().Jwt)
	return c, w
}

func readCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return int(body["code"].(float64))
}

func newProfileAPI() profile_api.ProfileApi {
	return profile_api.New(profile_api.Deps{
		DB:     testutil.DB(),
		JWT:    testutil.Config().Jwt,
		Logger: testutil.Logger(),
		Redis:  testutil.Redis(),
		System: testutil.Config().System,
	})
}

func TestProfileHandlers(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserStatModel{}, &models.UserViewDailyModel{}, &models.UserFollowModel{}, &models.TagModel{}, &models.UserSessionModel{}, &models.ArticleModel{}, &models.ImageModel{})
	_ = testutil.SetupMiniRedis(t)
	email := "u1@example.com"
	user := models.UserModel{
		Username: "u1",
		Password: "x",
		Email:    &email,
		Nickname: "nick",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	user.Avatar = "https://image.gentlybeing.cn/profile-u1.png"
	if err := db.Save(&user).Error; err != nil {
		t.Fatalf("更新用户头像失败: %v", err)
	}
	avatarImage := models.ImageModel{
		UserID:    user.ID,
		Provider:  enum.ImageProviderQiNiu,
		Bucket:    "myblogx",
		ObjectKey: "myblogx/images/profile-u1",
		FileName:  "profile-u1.png",
		URL:       user.Avatar,
		MimeType:  "image/png",
		Size:      1,
		Hash:      "hash-profile-u1",
		Status:    enum.ImageStatusPass,
	}
	if err := db.Create(&avatarImage).Error; err != nil {
		t.Fatalf("创建头像图片失败: %v", err)
	}
	viewer := models.UserModel{
		Username: "viewer",
		Password: "y",
		Nickname: "viewer",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("创建访客用户失败: %v", err)
	}
	if err := db.Create(&models.UserFollowModel{
		FollowedUserID: user.ID,
		FansUserID:     viewer.ID,
	}).Error; err != nil {
		t.Fatalf("创建用户关系失败: %v", err)
	}
	if err := db.Create(&models.ArticleModel{
		Title:         "a1",
		Content:       "content",
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Model(&models.UserStatModel{}).
		Where("user_id = ?", user.ID).
		Updates(map[string]any{
			"article_count":         1,
			"article_visited_count": 9,
		}).Error; err != nil {
		t.Fatalf("预置用户文章统计失败: %v", err)
	}

	tag := models.TagModel{Title: "Go", IsEnabled: true}
	disabledTag := models.TagModel{Title: "Hidden", IsEnabled: false}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("创建启用标签失败: %v", err)
	}
	if err := db.Create(&disabledTag).Error; err != nil {
		t.Fatalf("创建停用标签失败: %v", err)
	}
	if err := db.Model(&disabledTag).Update("is_enabled", false).Error; err != nil {
		t.Fatalf("更新停用标签状态失败: %v", err)
	}

	api := newProfileAPI()

	{
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		api.UserDetailView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("用户详情失败, code=%d body=%s", code, w.Body.String())
		}

		var body struct {
			Code int `json:"code"`
			Data struct {
				Email         *string  `json:"email"`
				HasPassword   bool     `json:"has_password"`
				AvatarImageID string   `json:"avatar_image_id"`
				LikeTagIDs    []string `json:"like_tag_ids"`
				LikeTagItems  []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"like_tag_items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析用户详情响应失败: %v", err)
		}
		if body.Data.Email == nil || *body.Data.Email != email {
			t.Fatalf("用户详情返回邮箱错误: %+v", body.Data.Email)
		}
		if !body.Data.HasPassword {
			t.Fatalf("用户详情应返回已绑定密码")
		}
		if body.Data.AvatarImageID != avatarImage.ID.String() {
			t.Fatalf("用户详情返回头像 image id 错误: %+v", body.Data.AvatarImageID)
		}
		if len(body.Data.LikeTagIDs) != 0 || len(body.Data.LikeTagItems) != 0 {
			t.Fatalf("初始偏好标签应为空: %+v", body.Data)
		}
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", models.IDRequest{ID: user.ID})
		api.UserBaseInfoView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("匿名获取用户基础信息失败, code=%d body=%s", code, w.Body.String())
		}

		var body struct {
			Code int `json:"code"`
			Data struct {
				AvatarImageID string `json:"avatar_image_id"`
				Abstract      string `json:"abstract"`
				Relation      int8   `json:"relation"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析匿名用户基础信息响应失败: %v", err)
		}
		if body.Data.Relation != 0 {
			t.Fatalf("匿名访问用户关系应为 0, got=%d", body.Data.Relation)
		}
		if body.Data.Abstract != user.Abstract {
			t.Fatalf("匿名访问用户简介异常: %+v", body.Data.Abstract)
		}
		if body.Data.AvatarImageID != avatarImage.ID.String() {
			t.Fatalf("匿名访问用户头像 image id 异常: %+v", body.Data.AvatarImageID)
		}
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", models.IDRequest{ID: user.ID})
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: viewer.ID, Role: viewer.Role}})
		api.UserBaseInfoView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("用户基础信息失败, code=%d body=%s", code, w.Body.String())
		}

		var body struct {
			Code int `json:"code"`
			Data struct {
				AvatarImageID       string   `json:"avatar_image_id"`
				Abstract            string   `json:"abstract"`
				ViewCount           int      `json:"view_count"`
				FansCount           int      `json:"fans_count"`
				FollowCount         int      `json:"follow_count"`
				ArticleVisitedCount int      `json:"article_visited_count"`
				ArticleCount        int      `json:"article_count"`
				FavoritesVisibility bool     `json:"favorites_visibility"`
				FollowVisibility    bool     `json:"followers_visibility"`
				FansVisibility      bool     `json:"fans_visibility"`
				HomeStyleID         ctype.ID `json:"home_style_id"`
				Relation            int8     `json:"relation"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析用户基础信息响应失败: %v", err)
		}
		if body.Data.ViewCount != 1 || body.Data.FansCount != 0 || body.Data.FollowCount != 0 || body.Data.ArticleVisitedCount != 9 || body.Data.ArticleCount != 1 {
			t.Fatalf("用户基础统计返回异常: %+v", body.Data)
		}
		if body.Data.Abstract != user.Abstract {
			t.Fatalf("用户基础简介返回异常: %+v", body.Data.Abstract)
		}
		if body.Data.AvatarImageID != avatarImage.ID.String() {
			t.Fatalf("用户基础头像 image id 返回异常: %+v", body.Data.AvatarImageID)
		}
		if !body.Data.FavoritesVisibility || !body.Data.FollowVisibility || !body.Data.FansVisibility {
			t.Fatalf("用户可见性默认值异常: %+v", body.Data)
		}
		if body.Data.HomeStyleID != 1 {
			t.Fatalf("用户首页样式默认值异常: %d", body.Data.HomeStyleID)
		}
		if int(body.Data.Relation) != 2 {
			t.Fatalf("用户关系字段异常: %d", body.Data.Relation)
		}

		var stat models.UserStatModel
		if err := db.Take(&stat, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("查询用户统计失败: %v", err)
		}
		if stat.ViewCount != 1 || stat.FansCount != 0 || stat.FollowCount != 0 || stat.ArticleVisitedCount != 9 || stat.ArticleCount != 1 {
			t.Fatalf("用户统计落库异常: %+v", stat)
		}

		var views []models.UserViewDailyModel
		if err := db.Where("user_id = ? AND viewer_user_id = ?", user.ID, viewer.ID).Find(&views).Error; err != nil {
			t.Fatalf("查询用户主页访问日记录失败: %v", err)
		}
		if len(views) != 1 {
			t.Fatalf("用户主页访问日记录数量异常: %d", len(views))
		}
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", models.IDRequest{ID: user.ID})
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: viewer.ID, Role: viewer.Role}})
		api.UserBaseInfoView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("重复访问用户基础信息失败, code=%d body=%s", code, w.Body.String())
		}

		var stat models.UserStatModel
		if err := db.Take(&stat, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("重复访问后查询用户统计失败: %v", err)
		}
		if stat.ViewCount != 1 {
			t.Fatalf("同一访客当天重复访问不应重复记数: %d", stat.ViewCount)
		}
	}

	{
		c, w := newCtx()
		newAvatar := models.ImageModel{
			UserID:    user.ID,
			Provider:  enum.ImageProviderQiNiu,
			Bucket:    "myblogx",
			ObjectKey: "myblogx/images/profile-u1-new",
			FileName:  "profile-u1-new.png",
			URL:       "https://image.gentlybeing.cn/profile-u1-new.png",
			MimeType:  "image/png",
			Size:      1,
			Hash:      "hash-profile-u1-new",
			Status:    enum.ImageStatusPass,
		}
		if err := db.Create(&newAvatar).Error; err != nil {
			t.Fatalf("创建新头像图片失败: %v", err)
		}
		newNick := "new-nick"
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		c.Set("requestJson", profile_api.UserInfoUpdateRequest{
			Nickname:      &newNick,
			AvatarImageID: &newAvatar.ID,
		})
		api.UserInfoUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("用户信息更新失败, code=%d body=%s", code, w.Body.String())
		}
		var updated models.UserModel
		if err := db.Take(&updated, user.ID).Error; err != nil {
			t.Fatalf("查询更新后的用户失败: %v", err)
		}
		if updated.Avatar != newAvatar.URL {
			t.Fatalf("头像应按 avatar_image_id 更新, got=%s want=%s", updated.Avatar, newAvatar.URL)
		}
	}

	{
		c, w := newCtx()
		favoritesVisibility := false
		followersVisibility := false
		fansVisibility := false
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		c.Set("requestJson", profile_api.UserInfoUpdateRequest{
			FavoritesVisibility: &favoritesVisibility,
			FollowVisibility:    &followersVisibility,
			FansVisibility:      &fansVisibility,
		})
		api.UserInfoUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("可见性配置更新失败, code=%d body=%s", code, w.Body.String())
		}

		var conf models.UserConfModel
		if err := db.Take(&conf, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("查询更新后的用户配置失败: %v", err)
		}
		if conf.FavoritesVisibility || conf.FollowVisibility || conf.FansVisibility {
			t.Fatalf("可见性配置更新结果异常: %+v", conf)
		}
	}

	{
		c, w := newCtx()
		likeTags := []ctype.ID{tag.ID, tag.ID, 0}
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		c.Set("requestJson", profile_api.UserInfoUpdateRequest{
			LikeTagIDs: &likeTags,
		})
		api.UserInfoUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("偏好标签更新失败, code=%d body=%s", code, w.Body.String())
		}

		var conf models.UserConfModel
		if err := db.Take(&conf, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("查询用户配置失败: %v", err)
		}
		if len(conf.LikeTags) != 1 || conf.LikeTags[0] != tag.ID {
			t.Fatalf("偏好标签去重结果异常: %+v", conf.LikeTags)
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		api.UserDetailView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新偏好标签后查询详情失败, code=%d body=%s", code, w.Body.String())
		}

		var body struct {
			Data struct {
				LikeTagIDs   []ctype.ID `json:"like_tag_ids"`
				LikeTagItems []struct {
					ID    ctype.ID `json:"id"`
					Title string   `json:"title"`
				} `json:"like_tag_items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析偏好标签详情失败: %v", err)
		}
		if len(body.Data.LikeTagIDs) != 1 || body.Data.LikeTagIDs[0] != tag.ID {
			t.Fatalf("详情返回的 like_tag_ids 异常: %+v", body.Data.LikeTagIDs)
		}
		if len(body.Data.LikeTagItems) != 1 || body.Data.LikeTagItems[0].ID != tag.ID || body.Data.LikeTagItems[0].Title != tag.Title {
			t.Fatalf("详情返回的 like_tag_items 异常: %+v", body.Data.LikeTagItems)
		}
	}

	{
		c, w := newCtx()
		likeTags := []ctype.ID{disabledTag.ID}
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		c.Set("requestJson", profile_api.UserInfoUpdateRequest{
			LikeTagIDs: &likeTags,
		})
		api.UserInfoUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("停用标签不应允许更新, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		role := enum.RoleAdmin
		c.Set("requestJson", profile_api.AdminUserInfoUpdateRequest{
			UserID: user.ID,
			Role:   &role,
		})
		api.AdminUserInfoUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("管理员更新用户失败, code=%d body=%s", code, w.Body.String())
		}
	}

	{
		// 覆盖用户名更新频率限制分支
		now := time.Now()
		_ = db.Model(&models.UserConfModel{}).Where("user_id = ?", user.ID).
			Update("updated_username_date", &now).Error
		c, w := newCtx()
		name := "u2_newname"
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role}})
		c.Set("requestJson", profile_api.UserInfoUpdateRequest{
			Username: &name,
		})
		api.UserInfoUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("用户名频率限制分支应失败, body=%s", w.Body.String())
		}
	}
}
