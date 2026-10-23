package article_api

import (
	"myblogx/api/article_api/favorite"
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOrCreateFavoriteIDBranches(t *testing.T) {
	user := setupArticleEnv(t)
	db := testutil.DB()

	fav, err := getOrCreateFavoriteID(db, 0, user.ID)
	if err != nil {
		t.Fatalf("默认收藏夹创建失败: %v", err)
	}
	if fav.ID == 0 || !fav.IsDefault {
		t.Fatalf("默认收藏夹数据异常: %+v", fav)
	}

	fav2, err := getOrCreateFavoriteID(db, 0, user.ID)
	if err != nil {
		t.Fatalf("默认收藏夹复用失败: %v", err)
	}
	if fav2.ID != fav.ID {
		t.Fatalf("默认收藏夹应复用同一条: %d vs %d", fav.ID, fav2.ID)
	}

	if _, err = getOrCreateFavoriteID(db, 9999, user.ID); err == nil {
		t.Fatal("不存在的收藏夹 ID 应报错")
	}
}

func TestFavoriteCreateUpdateListDelete(t *testing.T) {
	user := setupArticleEnv(t)
	db := testutil.DB()
	api := setupArticleAPI(t)
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser, Username: user.Username}}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", favorite.FavoriteRequest{
			Title:    "my favorite",
			Abstract: "desc",
			Cover:    "cover.png",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("创建收藏夹应成功, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", favorite.FavoriteRequest{
			Title:    "my favorite",
			Abstract: "desc2",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("重复标题应失败, body=%s", w.Body.String())
		}
	}

	var fav models.FavoriteModel
	if err := db.Where("user_id = ? and title = ?", user.ID, "my favorite").First(&fav).Error; err != nil {
		t.Fatalf("查询收藏夹失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", favorite.FavoriteRequest{
			ID:       fav.ID,
			Title:    "my favorite 2",
			Abstract: "desc3",
			Cover:    "c2.png",
		})
		api.FavoriteCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新收藏夹应成功, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestQuery", favorite.FavoriteListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     1,
		})
		req := httptest.NewRequest(http.MethodGet, "/favorites", nil)
		token := testutil.IssueAccessToken(t, user)
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
		req := httptest.NewRequest(http.MethodDelete, "/favorites", nil)
		token := testutil.IssueAccessToken(t, user)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteDeleteView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("空删除列表应失败, body=%s", w.Body.String())
		}
	}

	article := models.ArticleModel{
		Title:         "fav-delete-article",
		Content:       "content",
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Create(&models.UserArticleFavorModel{
		ArticleID: article.ID,
		UserID:    user.ID,
		FavorID:   fav.ID,
	}).Error; err != nil {
		t.Fatalf("创建收藏关联失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{fav.ID}})
		req := httptest.NewRequest(http.MethodDelete, "/favorites", nil)
		token := testutil.IssueAccessToken(t, user)
		req.Header.Set("token", token)
		c.Request = req
		api.FavoriteDeleteView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除收藏夹应成功, body=%s", w.Body.String())
		}
	}
}
