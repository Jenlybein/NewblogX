package middleware_test

import (
	"encoding/json"
	"myblogx/conf"
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

func newMiddlewareRuntime(t *testing.T, defaults conf.RuntimeDefaults) middleware.Runtime {
	t.Helper()
	baseCfg := testutil.Config()
	// testutil.DB() 只有一个槽位，同一用例里可能多次建引擎，而运行时配置表
	// 是进程级单例 —— 不清空的话第二次建引擎会读到上一次灌进去的旧值，
	// 配置看起来"改了不生效"。这里显式清空，模拟"这个引擎刚启动、库是空的"。
	if err := testutil.DB().Unscoped().Where("1 = 1").Delete(&schema.RuntimeConfigRow{}).Error; err != nil {
		t.Fatalf("清空运行时配置失败: %v", err)
	}
	// 日志默认值属于进程级配置，从这里补；其余段由用例自己给。
	defaults.Log = baseCfg.Log
	runtimeSvc := sitecfg.NewRuntimeConfigService(
		defaults,
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
	return newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{})
}

// newMiddlewareEngineWithSite 造一个把给定段灌进运行时配置表的引擎。
//
// 站点配置与验证码开关都已经在运行时配置里（不再来自 conf.Config），
// 所以"验证码开着"这类场景只能从默认值注入。参数是整包默认值而不是单个段：
// 验证码的五个开关在 captcha 段（票 481 从 site.login 迁入），站点段里没有它们了。
func newMiddlewareEngineWithSite(t *testing.T, defaults conf.RuntimeDefaults) *gin.Engine {
	t.Helper()
	runtimeMw := newMiddlewareRuntime(t, defaults)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("_middleware_runtime", runtimeMw)
		c.Next()
	})
	return r
}

func registerCaptchaEmailRoutes(r *gin.Engine) {
	// 验证码校验改成按入口挂闸门（第 12 段 / ADR-0033）：这里挂登录入口那一档。
	r.POST("/captcha", middleware.CaptchaGate(middleware.CaptchaScopeLogin), func(c *gin.Context) {
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

	r = newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{Captcha: true, CaptchaLogin: true}})
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

// captchaScopePaths 是四个入口各自的探测路径，用于下面的闸门矩阵。
func captchaScopePaths() []string {
	return []string{
		"/gate/" + middleware.CaptchaScopeLogin,
		"/gate/" + middleware.CaptchaScopeEmailVerify,
		"/gate/" + middleware.CaptchaScopePhoneVerify,
		// 注册专用入口：只有 POST /api/users/email/register 挂它。手机号注册没有独立
		// 路由（与登录共用 POST /api/users/phone），那条通道靠 phone_verify 兜。
		"/gate/" + middleware.CaptchaScopeRegister,
	}
}

// registerCaptchaGateRoutes 给三个入口各挂一条只观察"过没过"的路由。
//
// 路由的最终 handler 直接返回 200：本用例只关心闸门放行还是拦住，不关心
// 业务 handler 干了什么。未知 scope 那条单独挂，用来测"挂载点写错"的兜底。
func registerCaptchaGateRoutes(r *gin.Engine) {
	for _, path := range captchaScopePaths() {
		// 每条路由的 scope 从路径里取回来，避免闭包捕获同一个变量。
		scope := strings.TrimPrefix(path, "/gate/")
		r.POST(path, middleware.CaptchaGate(scope), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
	}
	r.POST("/gate/unknown_scope", middleware.CaptchaGate("unknown_scope"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}

// postCaptchaGate 发一次请求并返回响应，body 是 JSON 请求体。
func postCaptchaGate(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// 总开关关掉时，三个入口一个都不校验（哪怕各自的闸门都开着）。
//
// 总开关是"一键关掉"：如果关掉总开关但闸门还独立生效，那"先整体关、再逐条开"
// 的回滚路径就是假的 —— 运维以为自己已经关掉了验证码，登录页却还在 400，
// 而屏幕上没有任何东西说明为什么。
func TestCaptchaGateAllScopesOpenWhenMasterSwitchOff(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{
		Captcha:            false,
		CaptchaLogin:       true,
		CaptchaEmailVerify: true,
		CaptchaPhoneVerify: true,
		// 四个入口都要跟着矩阵走：漏一个，那个 scope 的用例就会把"闸门开"当成"闸门关"，
		// 于是断言成"放行"——一条永远绿的假用例。
		CaptchaRegister: true,
	}})
	registerCaptchaGateRoutes(r)

	for _, path := range captchaScopePaths() {
		// 故意不传 captcha_id：如果总开关真生效，这些请求不需要任何验证码就能过。
		if w := postCaptchaGate(t, r, path, `{}`); w.Code != http.StatusOK {
			t.Fatalf("%s：总开关关闭时应放行（不需要验证码），实际 code=%d body=%s",
				path, w.Code, w.Body.String())
		}
	}
}

// 总开关 + 闸门都打开时，缺 captcha_id 的请求必须被拒；带上正确验证码则放行。
//
// 两条断言必须成对出现：只断言"拒绝"的话，一个无条件拒绝的实现也能过——
// 那就成了"验证码永远填不对"的线上故障。带上正确码能过，才证明它真的在看 store。
//
// 拒绝的表现形式是业务码 1001（参数校验失败）+ HTTP 200：本仓库的成功/失败
// 一律走 HTTP 200 + 业务码（见 res.Response.Json），前端按 code 分支，所以
// 这里断言的是 1001 而不是 HTTP 400。
func TestCaptchaGateRejectsMissingCaptchaWhenSwitchesOn(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{
		Captcha:            true,
		CaptchaLogin:       true,
		CaptchaEmailVerify: true,
		CaptchaPhoneVerify: true,
		// 四个入口都要跟着矩阵走：漏一个，那个 scope 的用例就会把"闸门开"当成"闸门关"，
		// 于是断言成"放行"——一条永远绿的假用例。
		CaptchaRegister: true,
	}})
	registerCaptchaGateRoutes(r)

	for _, path := range captchaScopePaths() {
		w := postCaptchaGate(t, r, path, `{}`)
		if !hasBizCode(w, 1001) {
			t.Fatalf("%s：缺 captcha_id 时应以参数错误 1001 拒绝，实际 code 对不上 body=%s",
				path, w.Body.String())
		}
	}

	// 正确验证码必须能过（且每个入口用自己的 store 条目，避免"通过"来自上一轮的残留）。
	for _, path := range captchaScopePaths() {
		id := strings.TrimPrefix(path, "/gate/") + "-ok"
		_ = testutil.ImageCaptchaStore().Set(id, "1234")
		w := postCaptchaGate(t, r, path, `{"captcha_id":"`+id+`","captcha_code":"1234"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("%s：正确验证码应放行，实际 code=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

// 总开关开着但三个闸门都关着时，三个入口都不校验。
//
// 这条和上一条刚好互补：确认"校验"不是由总开关单独决定的。若实现退化成
// "总开关一开就全入口校验"，只想给发码加验证码的运营就只能连带把登录也卡住——
// 那正是这次把开关拆成三个闸门要解决的问题。
func TestCaptchaGateSkipsWhenScopeGateOff(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{
		Captcha:            true,
		CaptchaLogin:       false,
		CaptchaEmailVerify: false,
		CaptchaPhoneVerify: false,
	}})
	registerCaptchaGateRoutes(r)

	for _, path := range captchaScopePaths() {
		if w := postCaptchaGate(t, r, path, `{}`); w.Code != http.StatusOK {
			t.Fatalf("%s：闸门关闭时不该校验（不需要验证码），实际 code=%d body=%s",
				path, w.Code, w.Body.String())
		}
	}
}

// 未知 scope 一律放行，即使总开关和三个闸门都开着。
//
// 挂载点写错（拼了 "login " 或写了 "pwd_login"）时，宁可少校验一次 —— 那会立刻
// 被登录链路测出来；反过来把它当"要验证码"处理，会让一条没登记的路由静默
// 400，而错误信息完全不指向挂载点。
func TestCaptchaGateUnknownScopePasses(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{
		Captcha:            true,
		CaptchaLogin:       true,
		CaptchaEmailVerify: true,
		CaptchaPhoneVerify: true,
		// 四个入口都要跟着矩阵走：漏一个，那个 scope 的用例就会把"闸门开"当成"闸门关"，
		// 于是断言成"放行"——一条永远绿的假用例。
		CaptchaRegister: true,
	}})
	registerCaptchaGateRoutes(r)

	if w := postCaptchaGate(t, r, "/gate/unknown_scope", `{}`); w.Code != http.StatusOK {
		t.Fatalf("未知 scope 不该校验，实际 code=%d body=%s", w.Code, w.Body.String())
	}
}

// 三个闸门彼此独立：只打开登录那一档时，只有登录入口校验。
//
// 这是"按入口拆分"这个决定的全部价值。如果实现里三个 scope 互相牵连（例如
// 任一个开就全开），运营对发码限流的调整会连带改变登录页的体验，而没人能预测。
func TestCaptchaGateScopesAreIndependent(t *testing.T) {
	setupAuthEnv(t)
	r := newMiddlewareEngineWithSite(t, conf.RuntimeDefaults{Captcha: conf.Captcha{
		Captcha:            true,
		CaptchaLogin:       true,
		CaptchaEmailVerify: false,
		CaptchaPhoneVerify: false,
	}})
	registerCaptchaGateRoutes(r)

	if w := postCaptchaGate(t, r, "/gate/"+middleware.CaptchaScopeLogin, `{}`); !hasBizCode(w, 1001) {
		t.Fatalf("登录闸门开着时应校验，实际 body=%s", w.Body.String())
	}
	for _, path := range []string{
		"/gate/" + middleware.CaptchaScopeEmailVerify,
		"/gate/" + middleware.CaptchaScopePhoneVerify,
	} {
		if w := postCaptchaGate(t, r, path, `{}`); w.Code != http.StatusOK {
			t.Fatalf("%s：该入口闸门关着，不该跟着登录闸门一起校验，实际 code=%d body=%s",
				path, w.Code, w.Body.String())
		}
	}
}
