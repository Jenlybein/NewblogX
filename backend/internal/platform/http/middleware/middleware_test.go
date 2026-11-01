package middleware_test

import (
	"encoding/json"
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/kernel/enum"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/redis"
	redisEmail "myblogx/internal/platform/redis/redis_email"
	redisJWT "myblogx/internal/platform/redis/redis_jwt"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type bindReq struct {
	Name string `json:"name" form:"name" uri:"name" binding:"required"`
}

func TestBindMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/json", middleware.BindJson[bindReq], func(c *gin.Context) {
		req := middleware.GetBindJson[bindReq](c)
		c.JSON(http.StatusOK, gin.H{"name": req.Name})
	})
	r.GET("/query", middleware.BindQuery[bindReq], func(c *gin.Context) {
		req := middleware.GetBindQuery[bindReq](c)
		c.JSON(http.StatusOK, gin.H{"name": req.Name})
	})
	r.GET("/uri/:name", middleware.BindUri[bindReq], func(c *gin.Context) {
		req := middleware.GetBindUri[bindReq](c)
		c.JSON(http.StatusOK, gin.H{"name": req.Name})
	})

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"name":"alice"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("BindJson 状态码异常: %d", w.Code)
		}
	}
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/json", nil)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if !hasBizCode(w, 1002) {
			t.Fatalf("空 JSON 请求体应返回 1002, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "请求体不能为空，请在 Body 中传 JSON 参数") {
			t.Fatalf("空 JSON 请求体提示不明确, body=%s", w.Body.String())
		}
	}
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/query?name=bob", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("BindQuery 状态码异常: %d", w.Code)
		}
	}
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/uri/cindy", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("BindUri 状态码异常: %d", w.Code)
		}
	}
}

func setupAuthEnv(t *testing.T) {
	t.Helper()
	_ = testutil.SetupMiniRedis(t)
	_ = testutil.SetupSQLite(t, &schema.UserRow{}, &schema.RuntimeConfigRow{})
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "secret",
			Issuer: "issuer",
		},
	})
}

func testRedisOptions() redis_service.Options {
	return redis_service.Options{Client: testutil.Redis(), Logger: testutil.Logger()}
}

func newMiddlewareRuntime(t *testing.T, site conf.Site) middleware.Runtime {
	t.Helper()
	baseCfg := testutil.Config()
	// testutil.DB() 只有一个槽位，同一用例里可能多次建引擎，而运行时配置表
	// 是进程级单例 —— 不清空的话第二次建引擎会读到上一次灌进去的旧值，
	// 配置看起来"改了不生效"。这里显式清空，模拟"这个引擎刚启动、库是空的"。
	if err := testutil.DB().Unscoped().Where("1 = 1").Delete(&schema.RuntimeConfigRow{}).Error; err != nil {
		t.Fatalf("清空运行时配置失败: %v", err)
	}
	runtimeSvc := sitecfg.NewRuntimeConfigService(
		conf.RuntimeDefaults{Log: baseCfg.Log, Site: site},
		testutil.Logger(),
		testutil.DB(),
	)
	if err := runtimeSvc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化中间件运行时配置失败: %v", err)
	}
	return middleware.NewRuntime(middleware.RuntimeOptions{
		Verifier:          testutil.AuthVerifier{Config: testutil.Config().Jwt, Redis: testRedisOptions()},
		DB:                testutil.DB(),
		Redis:             testutil.Redis(),
		JWT:               testutil.Config().Jwt,
		Log:               testutil.Config().Log,
		Logger:            testutil.Logger(),
		RuntimeSite:       runtimeSvc,
		ImageCaptchaStore: testutil.ImageCaptchaStore(),
		System:            testutil.Config().System,
		ClickHouseConfig:  testutil.Config().ClickHouse,
	})
}

func newMiddlewareEngine(t *testing.T) *gin.Engine {
	return newMiddlewareEngineWithSite(t, conf.Site{})
}

// newMiddlewareEngineWithSite 造一个把站点段灌进运行时配置表的引擎。
//
// 站点配置（含登录验证码开关）已经是运行时配置，不再来自 conf.Config，
// 所以"验证码开着"这类场景只能从默认值注入。
func newMiddlewareEngineWithSite(t *testing.T, site conf.Site) *gin.Engine {
	t.Helper()
	runtimeMw := newMiddlewareRuntime(t, site)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("_middleware_runtime", runtimeMw)
		c.Next()
	})
	return r
}

func registerCaptchaEmailRoutes(r *gin.Engine) {
	r.POST("/captcha", middleware.CaptchaMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.POST("/email", middleware.EmailVerifyMiddleware, func(c *gin.Context) {
		email, _ := c.Get("email")
		c.JSON(http.StatusOK, gin.H{"email": email})
	})
}

func TestAuthAndAdminMiddleware(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngine(t)

	r.GET("/auth", middleware.AuthMiddleware, func(c *gin.Context) {
		claims := jwts.MustGetClaimsByGin(c)
		c.JSON(http.StatusOK, gin.H{"user": claims.Username})
	})
	r.GET("/admin", middleware.AuthMiddleware, middleware.AdminMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	user := &schema.UserRow{Username: "u1", Password: "x", Role: enum.RoleUser}
	admin := &schema.UserRow{Username: "admin", Password: "x", Role: enum.RoleAdmin}
	if err := testutil.DB().Create(user).Error; err != nil {
		t.Fatalf("创建普通用户失败: %v", err)
	}
	if err := testutil.DB().Create(admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	userToken := testutil.IssueAccessToken(t, user)
	adminToken := testutil.IssueAccessToken(t, admin)

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth", nil)
		req.Header.Set("token", userToken)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("AuthMiddleware 应通过, code=%d", w.Code)
		}
	}

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("token", userToken)
		r.ServeHTTP(w, req)
		if !hasBizCode(w, 1001) {
			t.Fatalf("普通用户访问 admin 应返回 1001, body=%s", w.Body.String())
		}
	}

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("token", adminToken)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("管理员应通过, code=%d", w.Code)
		}
	}

	{
		redisJWT.SetTokenBlack(testRedisOptions(), testutil.Config().Jwt, userToken, redisJWT.UserBlackType)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth", nil)
		req.Header.Set("token", userToken)
		r.ServeHTTP(w, req)
		// 鉴权失败用专属码 1003：前端据此判断要不要刷新令牌，而不是匹配文案。
		// （上面 /admin 那条是**权限**不足，仍走 1001，两者不应混淆。）
		if !hasBizCode(w, 1003) {
			t.Fatalf("黑名单 token 应被拦截（鉴权专属码 1003）, body=%s", w.Body.String())
		}
	}
}

func TestOptionalAuthMiddleware(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngine(t)

	r.GET("/optional-auth", middleware.OptionalAuthMiddleware, func(c *gin.Context) {
		if claims := jwts.GetClaimsByGin(c); claims != nil {
			c.JSON(http.StatusOK, gin.H{"user": claims.Username})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": ""})
	})

	user := &schema.UserRow{Username: "optional_user", Password: "x", Role: enum.RoleUser}
	if err := testutil.DB().Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	userToken := testutil.IssueAccessToken(t, user)

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/optional-auth", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("无 token 时 optional auth 不应拦截, code=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"user":""`) {
			t.Fatalf("无 token 时不应注入 claims, body=%s", w.Body.String())
		}
	}

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/optional-auth", nil)
		req.Header.Set("token", userToken)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("合法 token 时 optional auth 不应拦截, code=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), user.Username) {
			t.Fatalf("合法 token 时应注入 claims, body=%s", w.Body.String())
		}
	}

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/optional-auth", nil)
		req.Header.Set("token", "invalid-token")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("非法 token 时 optional auth 也不应拦截, code=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"user":""`) {
			t.Fatalf("非法 token 时不应注入 claims, body=%s", w.Body.String())
		}
	}
}

func TestCaptchaAndEmailVerifyMiddleware(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngine(t)
	registerCaptchaEmailRoutes(r)

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/captcha", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("验证码关闭时应通过, code=%d", w.Code)
		}
	}

	r = newMiddlewareEngineWithSite(t, conf.Site{Login: confsite.Login{Captcha: true}})
	registerCaptchaEmailRoutes(r)
	_ = testutil.ImageCaptchaStore().Set("cid", "1234")
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/captcha", strings.NewReader(`{"captcha_id":"cid","captcha_code":"1234"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("正确验证码应通过, code=%d", w.Code)
		}
	}

	_ = testutil.ImageCaptchaStore().Set("cid2", "5678")
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/captcha", strings.NewReader(`{"captcha_id":"cid2","captcha_code":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if !hasBizCode(w, 1001) {
			t.Fatalf("错误验证码应失败, body=%s", w.Body.String())
		}
	}

	if err := redisEmail.Store(testRedisOptions(), "eid", "u@example.com", "8888", 1, 3); err != nil {
		t.Fatalf("存储邮箱验证码失败: %v", err)
	}
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/email", strings.NewReader(`{"email_id":"eid","email_code":"8888"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("邮箱验证码应通过, code=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "u@example.com") {
			t.Fatalf("未写入 email 到 context, body=%s", w.Body.String())
		}
	}
}

func hasBizCode(w *httptest.ResponseRecorder, code int) bool {
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return false
	}
	v, ok := resp["code"]
	if !ok {
		return false
	}
	return int(v.(float64)) == code
}
