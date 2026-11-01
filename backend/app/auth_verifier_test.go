package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	authapp "myblogx/internal/auth/app"
	authapptest "myblogx/internal/auth/apptest"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/http/middleware"
	"myblogx/utils/jwts"
)

// 本文件锁死 ADR-0017 决策 3：**受限访客（guest / role=3）与未知角色（role=0）
// 在每请求重建身份时一律按"未登录"处理**。
//
// 为什么测在装配层、而不是各模块的 principal：
// 全站的"我是谁"都从这几个重建点来（中间件、文章 Viewer、搜索、私信……），
// 逐模块加判断必漏，而漏掉的地方不会报错——guest 只是再次拿回作者权限。
// 这里驱动的是生产同款装配：`authService` 延迟绑定 + 真实 `authVerifier`
// + 真实 AuthMiddleware / OptionalAuthMiddleware。
//
// 登录链路刻意不改（ADR 决策 3）：guest 仍能登录、仍能拿到令牌，
// 只是这张令牌在鉴权入口不被承认。

// roleToken 是一份"角色 + 已签发的访问令牌"夹具。
type roleToken struct {
	role     int8
	username string
	token    string
}

// seedRoleTokens 给 role=0/1/2/3 各登录一次，返回真令牌。
func seedRoleTokens(t *testing.T) ([]roleToken, *authapp.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repos := authapptest.NewRepos(time.Now())
	svc := repos.Service()
	ctx := context.Background()

	out := make([]roleToken, 0, 4)
	for _, role := range []int8{0, 1, 2, 3} {
		username := fmt.Sprintf("role%d", role)
		repos.Credentials().Seed(authapp.Account{
			ID:           ctype.NewID(uint64(100 + role)),
			Username:     username,
			Role:         role,
			TokenVersion: 1,
		}, "password123")

		res, err := svc.Login(ctx, authapp.LoginInput{Account: username, Password: "password123"})
		if err != nil {
			t.Fatalf("角色 %d 登录失败: %v", role, err)
		}
		out = append(out, roleToken{role: role, username: username, token: res.Tokens.Access})
	}
	return out, svc
}

// tokenOf 按角色取令牌。
func tokenOf(t *testing.T, tokens []roleToken, role int8) roleToken {
	t.Helper()
	for _, tk := range tokens {
		if tk.role == role {
			return tk
		}
	}
	t.Fatalf("夹具里没有 role=%d 的令牌", role)
	return roleToken{}
}

// useAuthService 把测试用的认证服务装到生产同款的延迟绑定点上，结束后还原。
//
// 不并发：authService 是包级单例，与生产一致；本文件的用例刻意不 t.Parallel。
func useAuthService(t *testing.T, svc *authapp.Service) {
	t.Helper()
	prev := authService
	authService = svc
	t.Cleanup(func() { authService = prev })
}

// newIdentityEngine 用真实 authVerifier 造两条代表路：需要登录的与公开的。
func newIdentityEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mwRuntime := middleware.NewRuntime(middleware.RuntimeOptions{Verifier: authVerifier{}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("_middleware_runtime", mwRuntime)
		c.Next()
	})

	// 需要登录的接口：访客在这里必须拿不到身份。
	r.GET("/mine", middleware.AuthMiddleware, func(c *gin.Context) {
		claims := jwts.MustGetClaimsByGin(c)
		c.JSON(http.StatusOK, gin.H{"username": claims.Username, "role": int8(claims.Role)})
	})

	// 公开接口：带令牌但不被承认时按匿名渲染。
	r.GET("/public", middleware.OptionalAuthMiddleware, func(c *gin.Context) {
		viewer := ""
		if claims := jwts.GetClaimsByGin(c); claims != nil {
			viewer = claims.Username
		}
		c.JSON(http.StatusOK, gin.H{"viewer": viewer})
	})
	return r
}

// envelope 是响应外壳（业务码在 body 里，HTTP 状态码恒为 200）。
type envelope struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data map[string]any `json:"data"`
}

// TestRestrictedRolesAreAnonymousOnHTTPRoutes 是验收标准"访客与未知角色的令牌
// 在需要登录的接口不被承认；公开接口按匿名处理"的 wiring 级用例。
func TestRestrictedRolesAreAnonymousOnHTTPRoutes(t *testing.T) {
	tokens, svc := seedRoleTokens(t)
	useAuthService(t, svc)
	engine := newIdentityEngine(t)

	cases := []struct {
		name     string
		role     int8
		loggedIn bool
	}{
		// role=0 是库里的列默认值（不在枚举里），与 guest 同待遇。
		{name: "未知角色 0 按未登录", role: 0, loggedIn: false},
		{name: "管理员 1 不变", role: 1, loggedIn: true},
		{name: "注册用户 2 不变", role: 2, loggedIn: true},
		{name: "受限访客 3 按未登录", role: 3, loggedIn: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := tokenOf(t, tokens, tc.role)

			// ── 需要登录的接口 ──
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/mine", nil)
			req.Header.Set("token", fixture.token)
			engine.ServeHTTP(w, req)

			if !tc.loggedIn {
				var got envelope
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
				}
				// 鉴权失败用专属码 1003（不再是 1001）：前端据此判断要不要
				// 刷新令牌，不需要再匹配错误消息里的中文关键词。
				if got.Code != 1003 {
					t.Fatalf("应被当成未登录（鉴权专属码 1003），实际 code=%d body=%s", got.Code, w.Body.String())
				}
				if got.Msg != "请先登录" {
					t.Fatalf("提示语应与匿名一致「请先登录」，实际 %q", got.Msg)
				}
				if _, ok := got.Data["username"]; ok {
					t.Fatalf("被拒时不应带出任何身份字段，body=%s", w.Body.String())
				}
			} else {
				var got struct {
					Username string `json:"username"`
					Role     int8   `json:"role"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
				}
				if got.Username != fixture.username {
					t.Fatalf("身份应为 %s，实际 %q（body=%s）", fixture.username, got.Username, w.Body.String())
				}
				if got.Role != tc.role {
					t.Fatalf("角色应为 %d，实际 %d", tc.role, got.Role)
				}
			}

			// ── 公开接口 ──
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodGet, "/public", nil)
			req.Header.Set("token", fixture.token)
			engine.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("公开接口不该被拦截，HTTP %d body=%s", w.Code, w.Body.String())
			}
			var pub struct {
				Viewer string `json:"viewer"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &pub); err != nil {
				t.Fatalf("解析公开接口响应失败: %v, body=%s", err, w.Body.String())
			}
			wantViewer := ""
			if tc.loggedIn {
				wantViewer = fixture.username
			}
			if pub.Viewer != wantViewer {
				t.Fatalf("公开接口的操作者应为 %q，实际 %q", wantViewer, pub.Viewer)
			}
		})
	}
}

// TestRestrictedRolesAreAnonymousInSearchAndChatBridges 覆盖另外两处"直接调
// Authenticate"的身份重建点：搜索的个性化视图与私信的 WebSocket 鉴权。
//
// 它们不走中间件，所以只改 authVerifier 盖不住：私信 WS 直接读 header 里的令牌，
// 漏掉这一处 guest 就能收发私信 —— 正是决策 3 要禁的"互动"。
func TestRestrictedRolesAreAnonymousInSearchAndChatBridges(t *testing.T) {
	tokens, svc := seedRoleTokens(t)
	ctx := context.Background()

	for _, role := range []int8{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("role=%d", role), func(t *testing.T) {
			fixture := tokenOf(t, tokens, role)
			allowed := role == int8(enum.RoleAdmin) || role == int8(enum.RoleUser)

			viewer, ok := (tokenVerifier{svc: svc}).Verify(ctx, fixture.token)
			if ok != allowed {
				t.Fatalf("搜索鉴权 ok=%v，期望 %v", ok, allowed)
			}
			if ok && viewer.Admin != (role == int8(enum.RoleAdmin)) {
				t.Fatalf("搜索鉴权的管理员标记不对: %+v", viewer)
			}

			_, _, isAdmin, ok := (chatAuthBridge{svc: svc}).Authenticate(ctx, fixture.token)
			if ok != allowed {
				t.Fatalf("私信鉴权 ok=%v，期望 %v", ok, allowed)
			}
			if ok && isAdmin != (role == int8(enum.RoleAdmin)) {
				t.Fatalf("私信鉴权的管理员标记不对: admin=%v", isAdmin)
			}
		})
	}
}

// TestDemotionTakesEffectOnTheNextRequest 证明"降级真的生效"：
// 角色每请求从库里重建，旧令牌里的 role 不参与判定。
func TestDemotionTakesEffectOnTheNextRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repos := authapptest.NewRepos(time.Now())
	svc := repos.Service()
	ctx := context.Background()

	account := authapp.Account{
		ID:           ctype.NewID(7),
		Username:     "demoted",
		Role:         int8(enum.RoleUser),
		TokenVersion: 1,
	}
	repos.Credentials().Seed(account, "password123")
	res, err := svc.Login(ctx, authapp.LoginInput{Account: account.Username, Password: "password123"})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	useAuthService(t, svc)
	if _, err := (authVerifier{}).VerifyAccessToken(ctx, res.Tokens.Access); err != nil {
		t.Fatalf("降级前令牌应可用: %v", err)
	}

	// 管理员把他改成受限访客；令牌版本不变，所以旧令牌本身仍然"有效"。
	account.Role = int8(enum.RoleGuest)
	repos.Credentials().Seed(account, "password123")

	_, err = (authVerifier{}).VerifyAccessToken(ctx, res.Tokens.Access)
	if err == nil {
		t.Fatal("降级为受限访客后，旧令牌不应再被承认")
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeAuthRequired {
		t.Fatalf("应报「请先登录」（auth.required），实际 %v", err)
	}
}
