package category

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

func readBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return body
}

func setupCategoryEnv(t *testing.T) *models.UserModel {
	t.Helper()
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.ArticleModel{},
	)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "category-secret",
			Issuer: "category-test",
		},
	})

	user := &models.UserModel{
		Username: "category_user",
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

func setupCategoryAPI() CategoryApi {
	return New(Deps{
		DB:     testutil.DB(),
		JWT:    testutil.Config().Jwt,
		Logger: testutil.Logger(),
		Redis:  testutil.Redis(),
	})
}

func TestCategoryCRUD(t *testing.T) {
	user := setupCategoryEnv(t)
	api := setupCategoryAPI()
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser, Username: user.Username}}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", CategoryRequest{Title: "后端分类"})
		api.CategoryCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("创建分类应成功, body=%s", w.Body.String())
		}
		data := readBody(t, w)["data"].(map[string]any)
		if _, ok := data["id"].(string); !ok {
			t.Fatalf("创建分类返回的 id 应为字符串, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", CategoryRequest{Title: "后端分类"})
		api.CategoryCreateUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("重复分类应失败, body=%s", w.Body.String())
		}
	}

	var cat models.CategoryModel
	if err := testutil.DB().Where("user_id = ? and title = ?", user.ID, "后端分类").First(&cat).Error; err != nil {
		t.Fatalf("查询分类失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", CategoryRequest{ID: cat.ID, Title: "后端分类-更新"})
		api.CategoryCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新分类应成功, body=%s", w.Body.String())
		}
	}

	token := tokenForUser(t, user)
	{
		c, w := newCtx()
		c.Set("requestQuery", CategoryListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Type:     1,
		})
		req := httptest.NewRequest(http.MethodGet, "/articles/category", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.CategoryListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("分类列表应成功, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{}})
		req := httptest.NewRequest(http.MethodDelete, "/articles/category", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.CategoryDeleteView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("空删除列表应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{cat.ID}})
		req := httptest.NewRequest(http.MethodDelete, "/articles/category", nil)
		req.Header.Set("token", token)
		c.Request = req
		api.CategoryDeleteView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除分类应成功, body=%s", w.Body.String())
		}
	}
}

func TestCategoryOptionsSupportsAnonymousAndAuthenticated(t *testing.T) {
	user := setupCategoryEnv(t)
	db := testutil.DB()
	api := setupCategoryAPI()
	otherUser := &models.UserModel{
		Username: "category_user_other",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(otherUser).Error; err != nil {
		t.Fatalf("创建第二个用户失败: %v", err)
	}

	privateCategory := models.CategoryModel{Title: "我的分类", UserID: user.ID}
	publicCategory := models.CategoryModel{Title: "公开分类", UserID: user.ID}
	otherSameTitleCategory := models.CategoryModel{Title: "公开分类", UserID: otherUser.ID}
	if err := db.Create(&[]models.CategoryModel{privateCategory, publicCategory, otherSameTitleCategory}).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	var categories []models.CategoryModel
	if err := db.Order("id asc").Find(&categories).Error; err != nil {
		t.Fatalf("回查分类失败: %v", err)
	}
	privateCategory = categories[0]
	publicCategory = categories[1]
	otherSameTitleCategory = categories[2]

	{
		c, w := newCtx()
		c.Set("requestQuery", CategoryOptionsRequest{UserID: user.ID})
		c.Request = httptest.NewRequest(http.MethodGet, "/articles/category/options", nil)
		api.CategoryOptionsView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("按用户获取分类选项应成功, body=%s", w.Body.String())
		}
		list := readBody(t, w)["data"].([]any)
		if len(list) != 2 {
			t.Fatalf("分类选项应只返回指定用户自己的分类, body=%s", w.Body.String())
		}
		first := list[0].(map[string]any)
		second := list[1].(map[string]any)
		if first["id"] != publicCategory.ID.String() || second["id"] != privateCategory.ID.String() {
			t.Fatalf("分类选项应按 title,id 排序且只返回指定用户分类, body=%s", w.Body.String())
		}
		if first["title"] != publicCategory.Title || second["title"] != privateCategory.Title {
			t.Fatalf("分类选项标题异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", CategoryOptionsRequest{UserID: otherUser.ID})
		c.Request = httptest.NewRequest(http.MethodGet, "/articles/category/options", nil)
		api.CategoryOptionsView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("按第二个用户获取分类选项应成功, body=%s", w.Body.String())
		}
		list := readBody(t, w)["data"].([]any)
		if len(list) != 1 {
			t.Fatalf("第二个用户分类选项应只返回自己的分类, body=%s", w.Body.String())
		}
		item := list[0].(map[string]any)
		if item["id"] != otherSameTitleCategory.ID.String() || item["title"] != otherSameTitleCategory.Title {
			t.Fatalf("分类选项不应混入其他用户同名分类, body=%s", w.Body.String())
		}
	}
}
