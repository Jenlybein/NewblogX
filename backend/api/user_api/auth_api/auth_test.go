package auth_api_test

import (
	"encoding/json"
	"myblogx/api/user_api/auth_api"
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/site_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"myblogx/utils/pwd"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func strPtr(s string) *string {
	return &s
}

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

func setupAuthEnv(t *testing.T) {
	t.Helper()
	testutil.InitGlobals()
	_ = testutil.SetupMiniRedis(t)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire:        1,
			RefreshExpire: 24,
			Secret:        "test-secret",
			Issuer:        "test",
		},
		Site: conf.Site{
			Login: confsite.Login{
				QQLogin:               false,
				UsernamePwdLogin:      false,
				EmailLogin:            false,
				EmailCodeTimeout:      5,
				LoginFailWindowMinute: 15,
				LoginFailUserMax:      5,
				LoginFailIPMax:        20,
				EmailSendWindowSecond: 60,
				EmailSendPerEmailMax:  1,
				EmailSendPerIPMax:     10,
			},
		},
	})
}

func newAuthAPI(t *testing.T) auth_api.AuthApi {
	t.Helper()
	runtimeSvc := site_service.NewRuntimeConfigService(testutil.Config().Site, testutil.Config().AI, testutil.Logger(), testutil.DB(), "")
	if err := runtimeSvc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化运行时配置失败: %v", err)
	}
	return auth_api.New(auth_api.Deps{
		DB:          testutil.DB(),
		Email:       testutil.Config().Email,
		JWT:         testutil.Config().Jwt,
		Logger:      testutil.Logger(),
		QQ:          testutil.Config().QQ,
		Redis:       testutil.Redis(),
		RuntimeSite: runtimeSvc,
		System:      testutil.Config().System,
	})
}

func TestAuthFeatureDisabledBranches(t *testing.T) {
	_ = testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	api := newAuthAPI(t)

	{
		c, w := newCtx()
		c.Set("requestJson", auth_api.PwdLoginRequest{Username: "u", Password: "p"})
		api.PwdLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("关闭密码登录时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestJson", auth_api.SendEmailRequest{Type: 1, Email: "u@example.com"})
		api.SendEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("关闭邮箱功能时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestJson", auth_api.RegisterEmailRequest{Pwd: "123456"})
		c.Set("email", "u@example.com")
		api.RegisterEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("关闭邮箱注册时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("email", "u@example.com")
		api.EmailLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("关闭邮箱登录时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestJson", auth_api.QQLoginRequest{Code: "abc"})
		api.QQLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("关闭 QQ 登录时应失败, body=%s", w.Body.String())
		}
	}
}

func TestResetUpdateBindEmail(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	api := newAuthAPI(t)

	hashPwd, _ := pwd.GenerateFromPassword("oldpwd")
	user := models.UserModel{
		Username: "u1",
		Password: hashPwd,
		Email:    strPtr("old@example.com"),
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("email", *user.Email)
		c.Set("requestJson", auth_api.ResetPasswordRequest{NewPassword: "newpwd"})
		api.ResetPwdByEmailView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("重置密码失败, code=%d body=%s", code, w.Body.String())
		}
	}

	{
		var got models.UserModel
		_ = db.First(&got, user.ID).Error
		if !pwd.CompareHashAndPassword(got.Password, "newpwd") {
			t.Fatal("重置后密码不正确")
		}
	}

	{
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		c.Set("requestJson", auth_api.UpdatePasswordRequest{
			OldPassword: "newpwd",
			NewPassword: "newpwd2",
		})
		api.UpdatePwdByEmailView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("修改密码失败, code=%d body=%s", code, w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("email", "bind@example.com")
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		api.BindEmailView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("绑定邮箱失败, code=%d body=%s", code, w.Body.String())
		}
	}
}
