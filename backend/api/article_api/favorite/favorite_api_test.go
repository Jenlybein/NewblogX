package favorite

import (
	"encoding/json"
	"myblogx/common"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
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

func setupFavoriteEnv(t *testing.T) *models.UserModel {
	t.Helper()
	_ = testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.FavoriteModel{},
		&models.UserArticleFavorModel{},
		&models.ArticleModel{},
		&models.ImageRefModel{},
	)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "favorite-secret",
			Issuer: "favorite-test",
		},
	})

	user := &models.UserModel{
		Username: "favorite_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return user
}

func tokenForUser(t *testing.T, user *models.UserModel) string {
	t.Helper()
	return testutil.IssueAccessToken(t, user)
}

func setupFavoriteAPI() FavoriteApi {
	return New(Deps{
		DB:     testutil.DB(),
		JWT:    testutil.Config().Jwt,
		Logger: testutil.Logger(),
		Redis:  testutil.Redis(),
	})
}

func readData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	return data
}

func TestFavoriteCRUD(t *testing.T) {
	user := setupFavoriteEnv(t)
	api := setupFavoriteAPI()
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser, Username: user.Username}}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", FavoriteRequest{
			Title:    "默认收藏组",
			Abstract: "desc",
			Cover:    "cover.png",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("创建收藏夹应成功, body=%s", w.Body.String())
		}
		data := readData(t, w)
		if got := data["id"]; got == nil || got == "" {
			t.Fatalf("创建收藏夹应返回字符串 id, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", FavoriteRequest{
			Title:    "默认收藏组",
			Abstract: "desc2",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("重复收藏夹应失败, body=%s", w.Body.String())
		}
	}

	var fav models.FavoriteModel
	if err := testutil.DB().Where("user_id = ? and title = ?", user.ID, "默认收藏组").First(&fav).Error; err != nil {
		t.Fatalf("查询收藏夹失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", FavoriteRequest{
			ID:       fav.ID,
			Title:    "默认收藏组更新",
			Abstract: "desc3",
			Cover:    "new.png",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新收藏夹应成功, body=%s", w.Body.String())
		}
	}

	token := tokenForUser(t, user)
	{
		c, w := newCtx()
		c.Set("requestQuery", FavoriteListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     1,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/favorite", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("收藏夹列表应成功, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{}})
		req := httptest.NewRequest(http.MethodDelete, "/articles/favorite", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteDeleteView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("空删除列表应失败, body=%s", w.Body.String())
		}
	}

	article := models.ArticleModel{
		Title:         "favorite-delete-article",
		Content:       "content",
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserArticleFavorModel{
		ArticleID: article.ID,
		UserID:    user.ID,
		FavorID:   fav.ID,
	}).Error; err != nil {
		t.Fatalf("创建收藏关联失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", FavoriteListRequest{
			PageInfo:  common.PageInfo{Page: 1, Limit: 10},
			Type:      1,
			ArticleID: article.ID,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/favorite", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("带文章状态的收藏夹列表应成功, body=%s", w.Body.String())
		}
		data := readData(t, w)
		list := data["list"].([]any)
		if len(list) == 0 || !list[0].(map[string]any)["has_article"].(bool) {
			t.Fatalf("收藏夹应返回 has_article=true, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{fav.ID}})
		req := httptest.NewRequest(http.MethodDelete, "/articles/favorite", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteDeleteView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除收藏夹应成功, body=%s", w.Body.String())
		}
	}
}

func TestFavoriteArticlesView(t *testing.T) {
	owner := setupFavoriteEnv(t)
	db := testutil.DB()
	api := setupFavoriteAPI()

	visitor := &models.UserModel{
		Username: "visitor_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(visitor).Error; err != nil {
		t.Fatalf("创建访客用户失败: %v", err)
	}

	favoriteModel := models.FavoriteModel{
		UserID:   owner.ID,
		Title:    "我的收藏夹",
		Abstract: "desc",
	}
	if err := db.Create(&favoriteModel).Error; err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	if err := db.Model(&models.UserConfModel{}).
		Where("user_id = ?", owner.ID).
		Update("favorites_visibility", false).Error; err != nil {
		t.Fatalf("设置收藏夹为私有失败: %v", err)
	}

	article1 := models.ArticleModel{
		Title:         "Go 文章",
		Abstract:      "go",
		Content:       "content",
		AuthorID:      owner.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	article2 := models.ArticleModel{
		Title:         "Redis 实战",
		Abstract:      "redis",
		Content:       "content",
		AuthorID:      owner.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := db.Create(&article1).Error; err != nil {
		t.Fatalf("创建文章1失败: %v", err)
	}
	if err := db.Create(&article2).Error; err != nil {
		t.Fatalf("创建文章2失败: %v", err)
	}
	if err := db.Create(&models.UserArticleFavorModel{
		ArticleID: article1.ID,
		UserID:    owner.ID,
		FavorID:   favoriteModel.ID,
	}).Error; err != nil {
		t.Fatalf("创建收藏关系1失败: %v", err)
	}
	if err := db.Create(&models.UserArticleFavorModel{
		ArticleID: article2.ID,
		UserID:    owner.ID,
		FavorID:   favoriteModel.ID,
	}).Error; err != nil {
		t.Fatalf("创建收藏关系2失败: %v", err)
	}

	ownerToken := tokenForUser(t, owner)
	visitorToken := tokenForUser(t, visitor)

	{
		c, w := newCtx()
		c.Set("requestQuery", FavoriteArticlesRequest{
			PageInfo:   common.PageInfo{Page: 1, Limit: 10, Key: "Go"},
			FavoriteID: favoriteModel.ID,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/favorite/articles", nil)
		req.Header.Set("token", ownerToken)
		c.Request = req
		api.FavoriteArticlesView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("收藏夹文章列表应成功, body=%s", w.Body.String())
		}
		data := readData(t, w)
		if data["has_more"].(bool) {
			t.Fatalf("关键词筛选后 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 1 {
			t.Fatalf("关键词筛选结果异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", FavoriteArticlesRequest{
			PageInfo:   common.PageInfo{Page: 1, Limit: 10},
			FavoriteID: favoriteModel.ID,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/favorite/articles", nil)
		req.Header.Set("token", visitorToken)
		c.Request = req
		api.FavoriteArticlesView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("私有收藏夹不应允许他人访问, body=%s", w.Body.String())
		}
	}

	if err := db.Model(&models.UserConfModel{}).
		Where("user_id = ?", owner.ID).
		Update("favorites_visibility", true).Error; err != nil {
		t.Fatalf("更新收藏夹可见性失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", FavoriteArticlesRequest{
			PageInfo:   common.PageInfo{Page: 1, Limit: 10},
			FavoriteID: favoriteModel.ID,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/favorite/articles", nil)
		req.Header.Set("token", visitorToken)
		c.Request = req
		api.FavoriteArticlesView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("公开收藏夹应允许访问, body=%s", w.Body.String())
		}
		data := readData(t, w)
		if data["has_more"].(bool) {
			t.Fatalf("公开收藏夹 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 2 {
			t.Fatalf("公开收藏夹文章数量异常, body=%s", w.Body.String())
		}
	}
}

func TestFavoriteListViewType2Visibility(t *testing.T) {
	owner := setupFavoriteEnv(t)
	db := testutil.DB()
	api := setupFavoriteAPI()

	favoriteModel := models.FavoriteModel{
		UserID:   owner.ID,
		Title:    "公开收藏夹测试",
		Abstract: "desc",
	}
	if err := db.Create(&favoriteModel).Error; err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	if err := db.Model(&models.UserConfModel{}).
		Where("user_id = ?", owner.ID).
		Update("favorites_visibility", false).Error; err != nil {
		t.Fatalf("设置收藏夹为私有失败: %v", err)
	}

	t.Run("未传 user_id 应失败", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestQuery", FavoriteListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     2,
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/articles/favorite", nil)
		api.FavoriteListView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("未传 user_id 应失败, body=%s", w.Body.String())
		}
	})

	t.Run("私有收藏夹不应公开访问", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestQuery", FavoriteListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     2,
			UserID:   owner.ID,
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/articles/favorite", nil)
		api.FavoriteListView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("私有收藏夹不应公开访问, body=%s", w.Body.String())
		}
	})

	if err := db.Model(&models.UserConfModel{}).
		Where("user_id = ?", owner.ID).
		Update("favorites_visibility", true).Error; err != nil {
		t.Fatalf("更新收藏夹可见性失败: %v", err)
	}

	t.Run("公开收藏夹允许匿名访问", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestQuery", FavoriteListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     2,
			UserID:   owner.ID,
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/articles/favorite", nil)
		api.FavoriteListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("公开收藏夹应允许访问, body=%s", w.Body.String())
		}
	})
}

func TestFavoriteRemovePatchView(t *testing.T) {
	user := setupFavoriteEnv(t)
	db := testutil.DB()
	api := setupFavoriteAPI()

	otherUser := &models.UserModel{
		Username: "favorite_other_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(otherUser).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	favoriteOne := models.FavoriteModel{UserID: user.ID, Title: "收藏夹一", Abstract: "desc"}
	favoriteTwo := models.FavoriteModel{UserID: user.ID, Title: "收藏夹二", Abstract: "desc"}
	if err := db.Create(&favoriteOne).Error; err != nil {
		t.Fatalf("创建收藏夹一失败: %v", err)
	}
	if err := db.Create(&favoriteTwo).Error; err != nil {
		t.Fatalf("创建收藏夹二失败: %v", err)
	}

	articles := []models.ArticleModel{
		{Title: "a1", Content: "content", AuthorID: user.ID, PublishStatus: enum.ArticleStatusPublished},
		{Title: "a2", Content: "content", AuthorID: user.ID, PublishStatus: enum.ArticleStatusPublished},
		{Title: "a3", Content: "content", AuthorID: user.ID, PublishStatus: enum.ArticleStatusPublished},
	}
	if err := db.Create(&articles).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	relations := []models.UserArticleFavorModel{
		{ArticleID: articles[0].ID, UserID: user.ID, FavorID: favoriteOne.ID},
		{ArticleID: articles[1].ID, UserID: user.ID, FavorID: favoriteOne.ID},
		{ArticleID: articles[2].ID, UserID: user.ID, FavorID: favoriteTwo.ID},
	}
	if err := db.Create(&relations).Error; err != nil {
		t.Fatalf("创建收藏关系失败: %v", err)
	}

	t.Run("无权限用户不能批量取消收藏", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:   otherUser.ID,
			Role:     otherUser.Role,
			Username: otherUser.Username,
		}})
		c.Set("requestJson", FavoriteRemovePatchModel{
			FavoriteID: favoriteOne.ID,
			Articles:   []ctype.ID{articles[0].ID},
		})
		api.FavoriteRemovePatchView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("无权限用户操作应失败, body=%s", w.Body.String())
		}
	})

	t.Run("批量取消收藏只移除当前收藏夹文章", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     user.Role,
			Username: user.Username,
		}})
		c.Set("requestJson", FavoriteRemovePatchModel{
			FavoriteID: favoriteOne.ID,
			Articles:   []ctype.ID{articles[0].ID, articles[1].ID, articles[2].ID},
		})
		api.FavoriteRemovePatchView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("批量取消收藏应成功, body=%s", w.Body.String())
		}

		var remainInFavoriteOne int64
		if err := db.Model(&models.UserArticleFavorModel{}).
			Where("favor_id = ?", favoriteOne.ID).
			Count(&remainInFavoriteOne).Error; err != nil {
			t.Fatalf("查询收藏夹一剩余关系失败: %v", err)
		}
		if remainInFavoriteOne != 0 {
			t.Fatalf("收藏夹一中的文章应全部移除, remain=%d", remainInFavoriteOne)
		}

		var remainInFavoriteTwo int64
		if err := db.Model(&models.UserArticleFavorModel{}).
			Where("favor_id = ? AND article_id = ?", favoriteTwo.ID, articles[2].ID).
			Count(&remainInFavoriteTwo).Error; err != nil {
			t.Fatalf("查询收藏夹二关系失败: %v", err)
		}
		if remainInFavoriteTwo != 1 {
			t.Fatalf("其他收藏夹关系不应被删除, remain=%d", remainInFavoriteTwo)
		}

		c2, w2 := newCtx()
		c2.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     user.Role,
			Username: user.Username,
		}})
		c2.Set("requestJson", FavoriteRemovePatchModel{
			FavoriteID: favoriteOne.ID,
			Articles:   []ctype.ID{articles[0].ID},
		})
		api.FavoriteRemovePatchView(c2)
		if code := readCode(t, w2); code == 0 {
			t.Fatalf("再次移除已软删关系应失败, body=%s", w2.Body.String())
		}
	})
}
