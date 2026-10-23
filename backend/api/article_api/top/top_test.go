package top

import (
	"encoding/json"
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

func newTopCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readTopBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return body
}

func readTopCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	body := readTopBody(t, w)
	return int(body["code"].(float64))
}

func setupTopEnv(t *testing.T) (models.UserModel, models.UserModel) {
	t.Helper()
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ArticleModel{},
		&models.UserTopArticleModel{},
	)

	user := models.UserModel{Username: "user_top", Password: "x", Role: enum.RoleUser}
	admin := models.UserModel{Username: "admin_top", Password: "x", Role: enum.RoleAdmin}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建普通用户失败: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	return user, admin
}

func createTopArticle(t *testing.T, authorID ctype.ID, title string, status enum.ArticleStatus) models.ArticleModel {
	t.Helper()
	article := models.ArticleModel{
		Title:         title,
		Content:       "content",
		AuthorID:      authorID,
		PublishStatus: status,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	return article
}

func setupTopAPI() TopApi {
	return New(Deps{
		DB:     testutil.DB(),
		Logger: testutil.Logger(),
		Redis:  testutil.Redis(),
	})
}

func TestArticleTopSetViewUserLimitAndAdminUnlimited(t *testing.T) {
	user, admin := setupTopEnv(t)
	api := setupTopAPI()

	userClaims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role, Username: user.Username}}
	adminClaims := &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Role: admin.Role, Username: admin.Username}}

	userArticles := []models.ArticleModel{
		createTopArticle(t, user.ID, "u1", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "u2", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "u3", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "u4", enum.ArticleStatusPublished),
	}

	for i := 0; i < 3; i++ {
		c, w := newTopCtx()
		c.Set("claims", userClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: userArticles[i].ID, Type: 1})
		api.ArticleTopSetView(c)
		if code := readTopCode(t, w); code != 0 {
			t.Fatalf("普通用户第 %d 次置顶失败, code=%d body=%s", i+1, code, w.Body.String())
		}
	}

	{
		c, w := newTopCtx()
		c.Set("claims", userClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: userArticles[3].ID, Type: 1})
		api.ArticleTopSetView(c)
		if code := readTopCode(t, w); code == 0 {
			t.Fatalf("普通用户第 4 次置顶应失败, body=%s", w.Body.String())
		}
	}

	adminArticles := []models.ArticleModel{
		createTopArticle(t, user.ID, "a1", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "a2", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "a3", enum.ArticleStatusPublished),
		createTopArticle(t, user.ID, "a4", enum.ArticleStatusExamining),
	}

	for i := range adminArticles {
		c, w := newTopCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: adminArticles[i].ID, Type: 2})
		api.ArticleTopSetView(c)
		if code := readTopCode(t, w); code != 0 {
			t.Fatalf("管理员第 %d 次置顶失败, code=%d body=%s", i+1, code, w.Body.String())
		}
	}

	var userCount int64
	if err := testutil.DB().Model(&models.UserTopArticleModel{}).Where("user_id = ?", user.ID).Count(&userCount).Error; err != nil {
		t.Fatalf("统计普通用户置顶数量失败: %v", err)
	}
	if userCount != 3 {
		t.Fatalf("普通用户置顶数量错误, got=%d want=3", userCount)
	}

	var adminCount int64
	if err := testutil.DB().Model(&models.UserTopArticleModel{}).Where("user_id = ?", admin.ID).Count(&adminCount).Error; err != nil {
		t.Fatalf("统计管理员置顶数量失败: %v", err)
	}
	if adminCount != 4 {
		t.Fatalf("管理员置顶数量错误, got=%d want=4", adminCount)
	}
}

func TestArticleTopRemoveViewRemovesOnlyCurrentUsersTop(t *testing.T) {
	user, admin := setupTopEnv(t)
	api := setupTopAPI()

	userClaims := &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: user.Role, Username: user.Username}}
	adminClaims := &jwts.MyClaims{Claims: jwts.Claims{UserID: admin.ID, Role: admin.Role, Username: admin.Username}}

	article := createTopArticle(t, user.ID, "shared-top", enum.ArticleStatusPublished)
	if err := testutil.DB().Create(&models.UserTopArticleModel{UserID: user.ID, ArticleID: article.ID}).Error; err != nil {
		t.Fatalf("创建用户置顶失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserTopArticleModel{UserID: admin.ID, ArticleID: article.ID}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}

	{
		c, w := newTopCtx()
		c.Set("claims", userClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: article.ID, Type: 1})
		api.ArticleTopRemoveView(c)
		if code := readTopCode(t, w); code != 0 {
			t.Fatalf("用户取消自己置顶失败, code=%d body=%s", code, w.Body.String())
		}
	}

	var count int64
	if err := testutil.DB().Model(&models.UserTopArticleModel{}).Where("article_id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatalf("统计剩余置顶失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("用户取消置顶后应只剩 1 条关系, got=%d", count)
	}

	{
		c, w := newTopCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: article.ID, Type: 2})
		api.ArticleTopRemoveView(c)
		if code := readTopCode(t, w); code != 0 {
			t.Fatalf("管理员取消管理员置顶失败, code=%d body=%s", code, w.Body.String())
		}
	}

	if err := testutil.DB().Model(&models.UserTopArticleModel{}).Where("article_id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatalf("统计最终置顶失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("全部取消置顶后关系应清空, got=%d", count)
	}

	{
		c, w := newTopCtx()
		c.Set("claims", userClaims)
		c.Set("requestJson", ArticleTopSetRequest{ArticleID: article.ID, Type: 1})
		api.ArticleTopSetView(c)
		if code := readTopCode(t, w); code != 0 {
			t.Fatalf("软删后重新置顶应成功, code=%d body=%s", code, w.Body.String())
		}
	}

	if err := testutil.DB().Unscoped().Model(&models.UserTopArticleModel{}).Where("user_id = ? AND article_id = ?", user.ID, article.ID).Count(&count).Error; err != nil {
		t.Fatalf("统计恢复后的置顶关系失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("重新置顶应复用原关系, got=%d", count)
	}
}

func TestArticleTopListViewByAuthor(t *testing.T) {
	user, admin := setupTopEnv(t)
	api := setupTopAPI()

	article1 := createTopArticle(t, user.ID, "top-1", enum.ArticleStatusPublished)
	article2 := createTopArticle(t, user.ID, "top-2", enum.ArticleStatusPublished)
	other := createTopArticle(t, admin.ID, "other-top", enum.ArticleStatusPublished)

	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		},
		UserID:    user.ID,
		ArticleID: article1.ID,
	}).Error; err != nil {
		t.Fatalf("创建作者置顶失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC),
		},
		UserID:    user.ID,
		ArticleID: article2.ID,
	}).Error; err != nil {
		t.Fatalf("创建作者第二条置顶失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 10, 10, 0, 0, time.UTC),
		},
		UserID:    admin.ID,
		ArticleID: other.ID,
	}).Error; err != nil {
		t.Fatalf("创建其他置顶失败: %v", err)
	}

	c, w := newTopCtx()
	c.Set("requestQuery", ArticleTopListRequest{
		Type:   1,
		UserID: user.ID,
	})
	api.ArticleTopListView(c)

	body := readTopBody(t, w)
	if code := int(body["code"].(float64)); code != 0 {
		t.Fatalf("查询作者置顶列表失败, code=%d body=%s", code, w.Body.String())
	}

	data := body["data"].(map[string]any)
	list := data["list"].([]any)
	if len(list) != 2 {
		t.Fatalf("作者置顶列表数量错误, got=%d", len(list))
	}

	first := list[0].(map[string]any)
	if got := first["id"].(string); got != article2.ID.String() {
		t.Fatalf("作者置顶应按最新置顶时间倒序, got=%s want=%s", got, article2.ID.String())
	}
	if top, ok := first["user_top"].(bool); !ok || !top {
		t.Fatalf("作者置顶标记错误, got=%#v", first["user_top"])
	}
}

func TestArticleTopListViewByAdminDeduplicatesArticles(t *testing.T) {
	user, admin1 := setupTopEnv(t)
	api := setupTopAPI()

	admin2 := models.UserModel{Username: "admin_top_2", Password: "x", Role: enum.RoleAdmin}
	if err := testutil.DB().Create(&admin2).Error; err != nil {
		t.Fatalf("创建第二个管理员失败: %v", err)
	}

	article1 := createTopArticle(t, user.ID, "admin-top-1", enum.ArticleStatusPublished)
	article2 := createTopArticle(t, user.ID, "admin-top-2", enum.ArticleStatusPublished)

	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 11, 0, 0, 0, time.UTC),
		},
		UserID:    admin1.ID,
		ArticleID: article1.ID,
	}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 11, 5, 0, 0, time.UTC),
		},
		UserID:    admin1.ID,
		ArticleID: article2.ID,
	}).Error; err != nil {
		t.Fatalf("创建管理员第二条置顶失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserTopArticleModel{
		Model: models.Model{
			CreatedAt: time.Date(2026, 3, 22, 11, 10, 0, 0, time.UTC),
		},
		UserID:    admin2.ID,
		ArticleID: article1.ID,
	}).Error; err != nil {
		t.Fatalf("创建重复管理员置顶失败: %v", err)
	}

	c, w := newTopCtx()
	c.Set("requestQuery", ArticleTopListRequest{
		Type: 2,
	})
	api.ArticleTopListView(c)

	body := readTopBody(t, w)
	if code := int(body["code"].(float64)); code != 0 {
		t.Fatalf("查询管理员置顶列表失败, code=%d body=%s", code, w.Body.String())
	}

	data := body["data"].(map[string]any)
	if count := int(data["count"].(float64)); count != 2 {
		t.Fatalf("管理员置顶文章应去重后返回, got=%d", count)
	}

	list := data["list"].([]any)
	if len(list) != 2 {
		t.Fatalf("管理员置顶列表长度错误, got=%d", len(list))
	}

	first := list[0].(map[string]any)
	if got := first["id"].(string); got != article1.ID.String() {
		t.Fatalf("管理员置顶应按最新置顶时间倒序, got=%s want=%s", got, article1.ID.String())
	}
	if top, ok := first["admin_top"].(bool); !ok || !top {
		t.Fatalf("管理员置顶标记错误, got=%#v", first["admin_top"])
	}
}
