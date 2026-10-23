package tags

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

func setupTagEnv(t *testing.T) *models.UserModel {
	t.Helper()
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.TagModel{},
		&models.ArticleModel{},
		&models.ArticleTagModel{},
	)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "tag-secret",
			Issuer: "tag-test",
		},
	})

	admin := &models.UserModel{
		Username: "admin_user",
		Password: "x",
		Role:     enum.RoleAdmin,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	return admin
}

func tokenForUser(t *testing.T, user *models.UserModel) string {
	t.Helper()
	return testutil.IssueAccessToken(t, user)
}

func setupTagsAPI() TagsApi {
	return New(Deps{
		DB:     testutil.DB(),
		Logger: testutil.Logger(),
		Redis:  testutil.Redis(),
	})
}

func TestTagCRUDAndOptions(t *testing.T) {
	admin := setupTagEnv(t)
	api := setupTagsAPI()
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Role: enum.RoleAdmin, Username: admin.Username}}

	{
		c, w := newCtx()
		enabled := true
		c.Set("claims", claims)
		c.Set("requestJson", TagRequest{
			Title:     "Golang",
			Sort:      10,
			IsEnabled: &enabled,
		})
		api.TagCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("创建标签失败, body=%s", w.Body.String())
		}
		data := readBody(t, w)["data"].(map[string]any)
		if _, ok := data["id"].(string); !ok {
			t.Fatalf("创建标签返回的 id 应为字符串, body=%s", w.Body.String())
		}
	}

	var tag models.TagModel
	if err := testutil.DB().Where("title = ?", "Golang").First(&tag).Error; err != nil {
		t.Fatalf("查询标签失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestQuery", TagListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
		})
		api.TagListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("标签列表失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		req := httptest.NewRequest(http.MethodGet, "/articles/tags/options", nil)
		c.Request = req
		api.ArticleTagOptionsView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("标签选项失败, body=%s", w.Body.String())
		}
		list := readBody(t, w)["data"].([]any)
		if len(list) != 1 {
			t.Fatalf("标签选项数量异常, body=%s", w.Body.String())
		}
		item := list[0].(map[string]any)
		if item["title"] != "Golang" || item["label"] != "Golang" {
			t.Fatalf("标签选项 title/label 异常, body=%s", w.Body.String())
		}
		if item["id"] != tag.ID.String() || item["value"] != tag.ID.String() {
			t.Fatalf("标签选项 id/value 异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		disabled := false
		c.Set("claims", claims)
		c.Set("requestJson", TagRequest{
			ID:        tag.ID,
			Title:     "Go",
			IsEnabled: &disabled,
		})
		api.TagCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新标签失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{tag.ID}})
		api.TagDeleteView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除标签失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		enabled := true
		c.Set("claims", claims)
		c.Set("requestJson", TagRequest{
			Title:     "Go",
			Sort:      20,
			IsEnabled: &enabled,
		})
		api.TagCreateUpdateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("软删后同名标签不应恢复原记录, body=%s", w.Body.String())
		}
	}

	var deleted models.TagModel
	if err := testutil.DB().Unscoped().Take(&deleted, tag.ID).Error; err != nil {
		t.Fatalf("查询已删除标签失败: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatal("同名标签创建失败后，原标签应保持软删除状态")
	}

	var activeCount int64
	if err := testutil.DB().Model(&models.TagModel{}).Where("title = ?", "Go").Count(&activeCount).Error; err != nil {
		t.Fatalf("统计活跃标签失败: %v", err)
	}
	if activeCount != 0 {
		t.Fatalf("软删后同名标签不应被自动恢复, count=%d", activeCount)
	}
}

func TestTagUpdateKeepsArticleTagRelation(t *testing.T) {
	admin := setupTagEnv(t)
	api := setupTagsAPI()
	claims := &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Role: enum.RoleAdmin, Username: admin.Username}}

	tag := models.TagModel{Title: "Golang", IsEnabled: true}
	if err := testutil.DB().Create(&tag).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	article := models.ArticleModel{
		Title:         "article-with-tag",
		Content:       "content",
		AuthorID:      admin.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := testutil.DB().Create(&models.ArticleTagModel{ArticleID: article.ID, TagID: tag.ID}).Error; err != nil {
		t.Fatalf("创建文章标签关系失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("claims", claims)
		c.Set("requestJson", TagRequest{
			ID:    tag.ID,
			Title: "Go",
		})
		api.TagCreateUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新标签失败, body=%s", w.Body.String())
		}
	}

	var updated models.ArticleModel
	if err := testutil.DB().Preload("Tags").Take(&updated, article.ID).Error; err != nil {
		t.Fatalf("回查文章失败: %v", err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0].ID != tag.ID || updated.Tags[0].Title != "Go" {
		t.Fatalf("标签改名后文章标签关系异常: %+v", updated.Tags)
	}
}
