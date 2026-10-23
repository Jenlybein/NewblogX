package view_history

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

func setupViewHistoryEnv(t *testing.T) *models.UserModel {
	t.Helper()
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.ArticleModel{},
		&models.UserArticleViewHistoryModel{},
	)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "history-secret",
			Issuer: "history-test",
		},
	})

	user := &models.UserModel{
		Username: "history_user",
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

func setupViewHistoryAPI() ViewHistoryApi {
	return New(Deps{
		DB: testutil.DB(),
	})
}

func TestViewHistoryListAndDelete(t *testing.T) {
	user := setupViewHistoryEnv(t)
	api := setupViewHistoryAPI()
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser, Username: user.Username}}

	article := models.ArticleModel{
		Title:         "history article",
		Content:       "content",
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	if err := testutil.DB().Create(&models.UserArticleViewHistoryModel{
		ArticleID: article.ID,
		UserID:    user.ID,
	}).Error; err != nil {
		t.Fatalf("创建浏览记录失败: %v", err)
	}

	token := tokenForUser(t, user)

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestQuery", ArticleViewHistoryRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     1,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/history", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.ArticleViewHistoryView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("浏览记录列表应成功, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{article.ID}})
		req := httptest.NewRequest(http.MethodDelete, "/articles/history", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.ArticleViewHistoryRemoveView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除浏览记录应成功, body=%s", w.Body.String())
		}
	}

	var count int64
	if err := testutil.DB().Model(&models.UserArticleViewHistoryModel{}).
		Where("user_id = ? and article_id = ?", user.ID, article.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("查询浏览记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("浏览记录应已删除, count=%d", count)
	}
}
