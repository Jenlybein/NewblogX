package auth_api_test

import (
	"encoding/json"
	"myblogx/api/user_api/auth_api"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"myblogx/utils/pwd"
	"testing"
)

func TestPwdLoginAndSendEmailFailureBranches(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	testutil.Config().Site.Login.UsernamePwdLogin = true
	testutil.Config().Site.Login.EmailLogin = true
	api := newAuthAPI(t)

	hashPwd, _ := pwd.GenerateFromPassword("right-pwd")
	user := models.UserModel{
		Username: "u_pwd",
		Password: hashPwd,
		Email:    strPtr("u_pwd@example.com"),
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	t.Run("用户名不存在", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestJson", auth_api.PwdLoginRequest{Username: "missing", Password: "x"})
		api.PwdLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("用户名不存在应失败, body=%s", w.Body.String())
		}
	})

	t.Run("密码错误", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestJson", auth_api.PwdLoginRequest{Username: "u_pwd", Password: "wrong"})
		api.PwdLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("密码错误应失败, body=%s", w.Body.String())
		}
	})

	t.Run("发送邮箱默认类型分支", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestJson", auth_api.SendEmailRequest{Type: 9, Email: "a@example.com"})
		api.SendEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("未知发送类型应失败, body=%s", w.Body.String())
		}
	})
}

func TestEmailPasswordAndBindFailureBranches(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	testutil.Config().Site.Login.EmailLogin = true
	api := newAuthAPI(t)

	hashPwd, _ := pwd.GenerateFromPassword("same-old")
	user := models.UserModel{
		Username: "u_email",
		Password: hashPwd,
		Email:    strPtr("u_email@example.com"),
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	t.Run("邮箱注册缺少 email 上下文", func(t *testing.T) {
		c, w := newCtx()
		c.Set("requestJson", auth_api.RegisterEmailRequest{Pwd: "123456"})
		api.RegisterEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("缺少 email 上下文应失败, body=%s", w.Body.String())
		}
	})

	t.Run("邮箱登录缺少 email 上下文", func(t *testing.T) {
		c, w := newCtx()
		api.EmailLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("邮箱登录缺少 email 上下文应失败, body=%s", w.Body.String())
		}
	})

	t.Run("邮箱登录用户不存在", func(t *testing.T) {
		c, w := newCtx()
		c.Set("email", "missing@example.com")
		api.EmailLoginView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("邮箱登录用户不存在应失败, body=%s", w.Body.String())
		}
	})

	t.Run("重置密码用户不存在", func(t *testing.T) {
		c, w := newCtx()
		c.Set("email", "missing@example.com")
		c.Set("requestJson", auth_api.ResetPasswordRequest{NewPassword: "x"})
		api.ResetPwdByEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("重置不存在用户应失败, body=%s", w.Body.String())
		}
	})

	t.Run("重置密码新旧相同", func(t *testing.T) {
		c, w := newCtx()
		c.Set("email", *user.Email)
		c.Set("requestJson", auth_api.ResetPasswordRequest{NewPassword: "same-old"})
		api.ResetPwdByEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("重置同密码应失败, body=%s", w.Body.String())
		}
	})

	t.Run("修改密码用户未绑定邮箱", func(t *testing.T) {
		noEmailPwd, _ := pwd.GenerateFromPassword("old")
		noEmailUser := models.UserModel{
			Username: "u_no_email",
			Password: noEmailPwd,
			Email:    nil,
			Role:     enum.RoleUser,
		}
		if err := db.Create(&noEmailUser).Error; err != nil {
			t.Fatalf("创建未绑定邮箱用户失败: %v", err)
		}

		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: noEmailUser.ID, Role: enum.RoleUser}})
		c.Set("requestJson", auth_api.UpdatePasswordRequest{OldPassword: "old", NewPassword: "new"})
		api.UpdatePwdByEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("未绑定邮箱应失败, body=%s", w.Body.String())
		}
	})

	t.Run("修改密码旧密码错误", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		c.Set("requestJson", auth_api.UpdatePasswordRequest{OldPassword: "wrong", NewPassword: "new"})
		api.UpdatePwdByEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("旧密码错误应失败, body=%s", w.Body.String())
		}
	})

	t.Run("修改密码新旧相同", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		c.Set("requestJson", auth_api.UpdatePasswordRequest{OldPassword: "same-old", NewPassword: "same-old"})
		api.UpdatePwdByEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("新旧密码相同应失败, body=%s", w.Body.String())
		}
	})

	t.Run("绑定邮箱缺少 email 上下文", func(t *testing.T) {
		c, w := newCtx()
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		api.BindEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("绑定邮箱缺少 email 应失败, body=%s", w.Body.String())
		}
	})

	t.Run("绑定邮箱不允许重复", func(t *testing.T) {
		otherUser := models.UserModel{
			Username: "u_bind_other",
			Password: hashPwd,
			Email:    strPtr("used@example.com"),
			Role:     enum.RoleUser,
		}
		if err := db.Create(&otherUser).Error; err != nil {
			t.Fatalf("创建已绑定邮箱用户失败: %v", err)
		}

		c, w := newCtx()
		c.Set("email", "used@example.com")
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: user.ID, Role: enum.RoleUser}})
		api.BindEmailView(c)
		if code := readCode(t, w); code == 0 {
			t.Fatalf("绑定重复邮箱应失败, body=%s", w.Body.String())
		}
	})
}

func TestEmailLoginView(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserSessionModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	testutil.Config().Site.Login.EmailLogin = true
	api := newAuthAPI(t)

	user := models.UserModel{
		Username: "u_email_login",
		Password: "",
		Email:    strPtr("u_login@example.com"),
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建邮箱登录用户失败: %v", err)
	}

	c, w := newCtx()
	c.Set("email", *user.Email)
	api.EmailLoginView(c)
	if code := readCode(t, w); code != 0 {
		t.Fatalf("邮箱登录应成功, body=%s", w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	token, _ := body["data"].(string)
	if token == "" {
		t.Fatalf("邮箱登录返回的 access token 不能为空, body=%s", w.Body.String())
	}
	if _, err := jwts.ParseToken(testutil.Config().Jwt, token); err != nil {
		t.Fatalf("邮箱登录返回的 token 无法解析: %v", err)
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("邮箱登录应写入 refresh token cookie")
	}

	var sessionCount int64
	if err := db.Model(&models.UserSessionModel{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("查询会话失败: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("邮箱登录应创建 1 条会话, got=%d", sessionCount)
	}
}

func TestUserModelUniqueIndexes(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{})

	first := models.UserModel{
		Username: "unique_user_1",
		Email:    strPtr("same@example.com"),
		OpenID:   strPtr("openid-1"),
		Role:     enum.RoleUser,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("创建首个用户失败: %v", err)
	}

	t.Run("用户名唯一", func(t *testing.T) {
		user := models.UserModel{
			Username: "unique_user_1",
			Email:    strPtr("another@example.com"),
			OpenID:   strPtr("openid-2"),
			Role:     enum.RoleUser,
		}
		if err := db.Create(&user).Error; err == nil {
			t.Fatal("重复用户名应被唯一索引拦截")
		}
	})

	t.Run("邮箱唯一", func(t *testing.T) {
		user := models.UserModel{
			Username: "unique_user_2",
			Email:    strPtr("same@example.com"),
			OpenID:   strPtr("openid-3"),
			Role:     enum.RoleUser,
		}
		if err := db.Create(&user).Error; err == nil {
			t.Fatal("重复邮箱应被唯一索引拦截")
		}
	})

	t.Run("OpenID唯一", func(t *testing.T) {
		user := models.UserModel{
			Username: "unique_user_3",
			Email:    strPtr("third@example.com"),
			OpenID:   strPtr("openid-1"),
			Role:     enum.RoleUser,
		}
		if err := db.Create(&user).Error; err == nil {
			t.Fatal("重复 OpenID 应被唯一索引拦截")
		}
	})
}
