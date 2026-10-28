package router_test

// 管理端路由的鉴权门禁。
//
// # 为什么需要它
//
// `contract_routes_test.go` 只对账"契约里的接口在不在路由表上"，不关心"要不要管理员"。
// 谁把一条后台路由从 `adminGroup` 挪到 `group` 上（一个词的差别），契约对账照样全绿、
// 编译照过、vet 照过，而任何登录用户都能调它。
//
// 这类缺陷本项目已经有过同族先例：`user/adapter/http` 那 6 个幽灵端点（12 §7.1）——
// 注解与真实路由是两份独立事实，缺一条机械对账就会静默漂移。
//
// # 为什么用行为探测，而不是读代码
//
// gin 的 `Routes()` 只给"方法 + 路径 + 最终 handler 名"，**不暴露中间件链**，
// 没法静态断言"这条路由挂了 AdminMiddleware"。但可以发一个请求看它停在哪：
//   - 普通用户令牌 → 必须停在 AdminMiddleware，返回"权限错误"；
//   - 如果没挂，请求会继续走到真实 handler —— 本测试注入的是零值 handler，
//     它会立刻 panic 并被 gin.Recovery 兜成 500。
//
// 于是"500 / 非 JSON" 与 "权限错误" 的区别，正好等价于"有没有挂 AdminMiddleware"。
//
// # 它的边界（必须说清）
//
// 探测表是手写的，新增后台接口不会自动进来。为此配了一条完整性检查：
// 契约里凡是落在已知后台区域前缀下、且**不带路径参数**的 operation，必须登记在表里。
// 带参数的详情接口（`/logs/runtime/{id}` 这类）跳过 —— 它们与各自的列表接口在同一个
// group 里注册，且用字面量 `{id}` 去探测会先在路由层 404，测不到中间件。

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aihttp "myblogx/internal/ai/adapter/http"
	analyticshttp "myblogx/internal/analytics/adapter/http"
	"myblogx/internal/apperr"
	articlehttp "myblogx/internal/article/adapter/http"
	authhttp "myblogx/internal/auth/adapter/http"
	chathttp "myblogx/internal/chat/adapter/http"
	commenthttp "myblogx/internal/comment/adapter/http"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	mediahttp "myblogx/internal/media/adapter/http"
	notificationhttp "myblogx/internal/notification/adapter/http"
	"myblogx/internal/platform/captcha"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/router"
	searchhttp "myblogx/internal/search/adapter/http"
	sitehttp "myblogx/internal/site/adapter/http"
	socialhttp "myblogx/internal/social/adapter/http"
	userhttp "myblogx/internal/user/adapter/http"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// adminOnly 是"必须只有管理员能访问"的具体操作（不含路径参数）。
var adminOnly = []struct {
	method string
	path   string
}{
	// 数据看板
	{"GET", "/api/data/sum"},
	{"GET", "/api/data/growth"},
	{"GET", "/api/data/article-year"},
	// 日志中心（含死信查看与重投）
	{"GET", "/api/logs/runtime"},
	{"GET", "/api/logs/action"},
	{"GET", "/api/logs/login"},
	{"GET", "/api/logs/cdc"},
	{"GET", "/api/logs/replay"},
	{"GET", "/api/logs/chain"},
	{"GET", "/api/logs/dead-letter"},
	{"POST", "/api/logs/dead-letter/replay"},
	// 用户管理
	{"GET", "/api/users/admin/list"},
	{"POST", "/api/users/admin"},
	{"PUT", "/api/users/admin/info"},
	// 全局通知的写操作（读是登录用户都有的）
	{"POST", "/api/global_notif"},
	{"DELETE", "/api/global_notif"},
}

// adminAreaPrefixes 是"整段只给管理员"的路径前缀，用于完整性检查。
//
// 只收这类前缀：读写混在一起的区域（/site/{name}、/articles、/comments）
// 没法靠前缀判断，只能靠人在 adminOnly 里登记。
var adminAreaPrefixes = []string{
	"/data/",
	"/logs/",
	"/users/admin",
	"/site/admin/",
}

// userTokenVerifier 让任何非空令牌都以"普通用户"身份通过。
//
// 用 RoleUser 是刻意的：本测试要证明的正是"普通用户进不去"。
type userTokenVerifier struct{}

func (userTokenVerifier) VerifyAccessToken(_ context.Context, raw string) (*jwts.MyClaims, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	return &jwts.MyClaims{Claims: jwts.Claims{
		UserID:    ctype.NewID(1),
		SessionID: ctype.NewID(2),
		Role:      enum.RoleUser,
		Username:  "ordinary-user",
	}}, nil
}

// injectZeroHandlers 注入零值处理器：本测试只关心"停在哪一层"。
// 任何 handler 方法被真正调用都会 panic —— 那恰好说明中间件没拦住。
func injectZeroHandlers() {
	router.SetSiteHandler(&sitehttp.Handler{})
	router.SetAnalyticsHandler(&analyticshttp.Handler{})
	router.SetImageHandler(&mediahttp.Handler{})
	router.SetCaptchaHandler(&captcha.Handler{})
	router.SetAuthHandler(&authhttp.Handler{})
	router.SetUserHandler(&userhttp.Handler{})
	router.SetArticleHandler(&articlehttp.Handler{})
	router.SetCommentHandler(&commenthttp.Handler{})
	router.SetChatHandler(&chathttp.Handler{})
	router.SetNotificationHandler(&notificationhttp.Handler{})
	router.SetFollowHandler(&socialhttp.Handler{})
	router.SetSearchHandler(&searchhttp.Handler{})
	router.SetAIHandler(&aihttp.Handler{})
}

// TestAdminRoutesRejectOrdinaryUser 逐个探测：普通用户令牌必须被挡在 AdminMiddleware。
func TestAdminRoutesRejectOrdinaryUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 探测时会故意让未受保护的 handler panic，别让栈把测试输出刷满。
	gin.DefaultErrorWriter = io.Discard

	injectZeroHandlers()
	engine := router.NewEngine(mw.RuntimeOptions{Verifier: userTokenVerifier{}}, nil)

	for _, c := range adminOnly {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			req.Header.Set("Authorization", "Bearer ordinary-user-token")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			var body struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("响应不是 JSON（status=%d body=%q）：这条路由大概没挂 AdminMiddleware，"+
					"请求已经打到 handler 上了", rec.Code, rec.Body.String())
			}
			if body.Code != int(res.FailValidCode) || !strings.Contains(body.Msg, "权限") {
				t.Fatalf("普通用户应被拒绝（code=%d msg=权限错误），实际 status=%d code=%d msg=%q",
					int(res.FailValidCode), rec.Code, body.Code, body.Msg)
			}
		})
	}
}

// TestProbeDistinguishesUnprotectedRoute 证明上面那条探测**真的会红**。
//
// 门禁的教训（见 06 号文档）：一个永远通过的断言等于没有。这里拿一条明确的
// 非后台路由（登录用户都能看的自己的资料）做对照：它应当走到 handler 上、
// 因为零值 handler 而 panic 成 500 —— 而不是返回"权限错误"。
//
// 换句话说：如果哪天有人把 adminOnly 里的某条路由的 AdminMiddleware 摘掉，
// 探测会得到和这里一样的结果，于是那条用例会红。
func TestProbeDistinguishesUnprotectedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gin.DefaultErrorWriter = io.Discard

	injectZeroHandlers()
	engine := router.NewEngine(mw.RuntimeOptions{Verifier: userTokenVerifier{}}, nil)

	req := httptest.NewRequest("GET", "/api/users/detail", nil)
	req.Header.Set("Authorization", "Bearer ordinary-user-token")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil &&
		body.Code == int(res.FailValidCode) && strings.Contains(body.Msg, "权限") {
		t.Fatal("对照路由 /api/users/detail 不该要求管理员 —— 但如果真返回了权限错误，" +
			"说明探测本身分不清'拦住'与'放行'，上面的门禁是假绿")
	}
}

// TestAdminOnlyListCoversContract 完整性检查：契约里落在后台区域、且不带路径参数的
// operation 必须登记到 adminOnly，否则新增的后台接口会悄悄逃过鉴权探测。
func TestAdminOnlyListCoversContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "swagger.json"))
	if err != nil {
		t.Fatalf("读取契约失败: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct{} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("解析契约失败: %v", err)
	}

	registered := make(map[string]struct{}, len(adminOnly))
	for _, c := range adminOnly {
		registered[strings.ToUpper(c.method)+" "+c.path] = struct{}{}
	}

	for path := range spec.Paths {
		if strings.Contains(path, "{") {
			continue // 带参数的详情接口：见文件头"它的边界"
		}
		inArea := false
		for _, prefix := range adminAreaPrefixes {
			if strings.HasPrefix(path, prefix) {
				inArea = true
				break
			}
		}
		if !inArea {
			continue
		}
		for method := range spec.Paths[path] {
			op := strings.ToUpper(method) + " /api" + path
			if _, ok := registered[op]; !ok {
				t.Fatalf("契约里有后台接口 %s，但没登记到 adminOnly。\n"+
					"它是否真的要求管理员需要人来判断：要求就加进表里；不要求就把它从后台区域挪出去", op)
			}
		}
	}
}
