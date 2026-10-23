package user_man_api

import (
	"encoding/json"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"myblogx/utils/pwd"
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
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	return int(body["code"].(float64))
}

func readData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	data, _ := body["data"].(map[string]any)
	return data
}

func setupUserManAPI(t *testing.T) (*UserManApi, *models.UserModel) {
	t.Helper()
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserStatModel{})
	api := New(Deps{
		DB:     db,
		Logger: testutil.Logger(),
	})
	admin := &models.UserModel{
		Username: "admin_creator",
		Password: "x",
		Nickname: "管理员",
		Role:     enum.RoleAdmin,
	}
	testutil.CreateUser(t, db, admin)
	return &api, admin
}

func TestAdminUserCreateView(t *testing.T) {
	api, admin := setupUserManAPI(t)
	adminClaims := &jwts.MyClaims{Claims: jwts.Claims{
		UserID:   admin.ID,
		Role:     admin.Role,
		Username: admin.Username,
	}}

	t.Run("创建普通用户成功并返回摘要", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", AdminUserCreateRequest{
			Username: "created_user",
			Password: "secret123",
			Nickname: "新用户",
			Email:    "created_user@example.com",
		})
		api.AdminUserCreateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("管理员创建用户应成功 body=%s", w.Body.String())
		}

		data := readData(t, w)
		if got := data["id"]; got == nil || got == "" {
			t.Fatalf("创建响应应返回字符串 id body=%s", w.Body.String())
		}
		if got := int(data["role"].(float64)); got != int(enum.RoleUser) {
			t.Fatalf("创建用户角色应固定为普通用户 got=%d body=%s", got, w.Body.String())
		}
		if got := int(data["register_source"].(float64)); got != int(enum.RegisterAdminSourceType) {
			t.Fatalf("创建来源应标记为管理员创建 got=%d body=%s", got, w.Body.String())
		}

		var created models.UserModel
		if err := testutil.DB().Take(&created, "username = ?", "created_user").Error; err != nil {
			t.Fatalf("查询创建用户失败: %v", err)
		}
		if created.Role != enum.RoleUser {
			t.Fatalf("数据库用户角色错误: %d", created.Role)
		}
		if created.RegisterSource != enum.RegisterAdminSourceType {
			t.Fatalf("数据库注册来源错误: %d", created.RegisterSource)
		}
		if !pwd.CompareHashAndPassword(created.Password, "secret123") {
			t.Fatalf("密码应被正确加密")
		}

		var conf models.UserConfModel
		if err := testutil.DB().Take(&conf, "user_id = ?", created.ID).Error; err != nil {
			t.Fatalf("应初始化 user_conf: %v", err)
		}
		var stat models.UserStatModel
		if err := testutil.DB().Take(&stat, "user_id = ?", created.ID).Error; err != nil {
			t.Fatalf("应初始化 user_stat: %v", err)
		}
	})

	t.Run("昵称留空时默认使用用户名", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", AdminUserCreateRequest{
			Username: "default_nickname_user",
			Password: "secret123",
		})
		api.AdminUserCreateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("管理员创建用户应成功 body=%s", w.Body.String())
		}

		var created models.UserModel
		if err := testutil.DB().Take(&created, "username = ?", "default_nickname_user").Error; err != nil {
			t.Fatalf("查询创建用户失败: %v", err)
		}
		if created.Nickname != created.Username {
			t.Fatalf("昵称默认值错误 nickname=%s username=%s", created.Nickname, created.Username)
		}
	})

	t.Run("用户名重复时失败", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", AdminUserCreateRequest{
			Username: "created_user",
			Password: "secret123",
		})
		api.AdminUserCreateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("用户名重复应失败 body=%s", w.Body.String())
		}
	})

	t.Run("邮箱重复时失败", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", adminClaims)
		c.Set("requestJson", AdminUserCreateRequest{
			Username: "another_user",
			Password: "secret123",
			Email:    "created_user@example.com",
		})
		api.AdminUserCreateView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("邮箱重复应失败 body=%s", w.Body.String())
		}
	})
}

func TestUserListViewUsesAppDB(t *testing.T) {
	api, _ := setupUserManAPI(t)

	user := &models.UserModel{
		Username: "listed_user",
		Password: "x",
		Nickname: "列表用户",
		Role:     enum.RoleUser,
	}
	testutil.CreateUser(t, testutil.DB(), user)

	c, w := newCtx()
	c.Set("requestQuery", UserListRequest{})
	api.UserListView(c)

	if code := readCode(t, w); code != 0 {
		t.Fatalf("用户列表应成功 body=%s", w.Body.String())
	}
	data := readData(t, w)
	if got := int(data["count"].(float64)); got < 2 {
		t.Fatalf("用户列表应返回已创建用户 got=%d body=%s", got, w.Body.String())
	}
	list, _ := data["list"].([]any)
	if len(list) == 0 {
		t.Fatalf("用户列表不应为空 body=%s", w.Body.String())
	}
}
