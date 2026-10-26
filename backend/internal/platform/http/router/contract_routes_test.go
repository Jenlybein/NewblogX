package router_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	aihttp "myblogx/internal/ai/adapter/http"
	analyticshttp "myblogx/internal/analytics/adapter/http"
	articlehttp "myblogx/internal/article/adapter/http"
	authhttp "myblogx/internal/auth/adapter/http"
	chathttp "myblogx/internal/chat/adapter/http"
	commenthttp "myblogx/internal/comment/adapter/http"
	mediahttp "myblogx/internal/media/adapter/http"
	notificationhttp "myblogx/internal/notification/adapter/http"
	"myblogx/internal/platform/captcha"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
	searchhttp "myblogx/internal/search/adapter/http"
	sitehttp "myblogx/internal/site/adapter/http"
	socialhttp "myblogx/internal/social/adapter/http"
	userhttp "myblogx/internal/user/adapter/http"

	"github.com/gin-gonic/gin"
)

// 本文件把"契约"和"真实路由表"钉在一起。
//
// 起因：user 模块曾留下一整套没人调用的旧 handler（`handler.go` / `dto.go`），
// 它们的 swag 注解仍然生效，于是契约里长出了 6 个 operation——
// `GET|POST|PUT /users`、`GET /users/{id}`、`PUT /users/settings`、
// `PUT /users/{id}/status`——运行时全部 404。
// 编译、vet、archcheck、覆盖率、契约生成**没有任何一环**能发现这件事：
// 注解写在一份死代码上，而契约只认注解。
//
// 所以这里做双向对账，两个方向的含义完全不同：
//
//	方向一（严格）：契约里每个 operation 必须真的注册在路由表上。
//	               不满足 = 契约在说谎，前端照着它写的调用会 404。这是缺陷，必须修。
//	方向二（棘轮）：路由表里没进契约的端点必须列在 allowNotInContract 里。
//	               不满足 = 新增了没有文档化的接口；或者反过来，注解被误删。
//	               这类不一定是缺陷（健康检查、静态资源、WS 升级本就不该进契约），
//	               所以要显式登记理由，而不是一律放过。
//
// 为什么用 NewEngine 而不是在测试里再写一遍挂载顺序：见 enter.go 里 NewEngine 的注释。

// allowNotInContract 是"确实不该出现在 OpenAPI 契约里"的路由白名单。
// 每一条都要写清理由——这张表的目的是让缺口可见，不是让测试闭嘴。
var allowNotInContract = map[string]string{
	"GET /health":              "健康检查，基础设施探针用，不属于业务 API",
	"GET /uploads/{filepath}":  "静态资源（gin.Static），由 nginx 兜底，不是接口",
	"HEAD /uploads/{filepath}": "同上，HEAD 是 gin 为静态目录自动注册的",
}

// TestContractMatchesRoutes 校验 docs/swagger.json 与真实路由表一致。
func TestContractMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 注入零值处理器：本测试只关心路由**注册**，任何 handler 方法都不会被调用。
	// 不用 NewHandler 是因为它们会校验依赖非空（那属于 handler 自己的测试），
	// 而这里正需要"没有依赖的处理器"。
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

	engine := router.NewEngine(mw.RuntimeOptions{})
	routes := engine.Routes()
	if len(routes) == 0 {
		t.Fatal("路由表为空：装配路径没有注册任何路由")
	}

	registered := make(map[string]bool, len(routes))
	for _, rt := range routes {
		registered[normalizeRoute(rt.Method, rt.Path)] = true
	}

	contracted, err := readContractOps()
	if err != nil {
		t.Fatalf("读取契约失败: %v", err)
	}
	if len(contracted) == 0 {
		t.Fatal("契约里没有任何 operation：swagger.json 可能被清空或生成失败")
	}

	// 方向一：契约 → 路由表。这是"契约说谎"，属于缺陷。
	var phantom []string
	for _, op := range contracted {
		if !registered[op] {
			phantom = append(phantom, op)
		}
	}
	if len(phantom) > 0 {
		sort.Strings(phantom)
		t.Errorf("契约里有 %d 个 operation 在路由表上不存在（运行时必然 404）：\n  %s\n"+
			"修法：删掉对应 handler 方法（注解随之消失）并重新生成契约，或把它真正注册进路由。",
			len(phantom), strings.Join(phantom, "\n  "))
	}

	// 方向二：路由表 → 契约。这是"文档缺口"，允许登记例外。
	inContract := make(map[string]bool, len(contracted))
	for _, op := range contracted {
		inContract[op] = true
	}
	var undocumented []string
	for op := range registered {
		if inContract[op] {
			continue
		}
		if _, ok := allowNotInContract[op]; !ok {
			undocumented = append(undocumented, op)
		}
	}
	if len(undocumented) > 0 {
		sort.Strings(undocumented)
		t.Errorf("有 %d 个路由既不在契约里、也没登记到白名单：\n  %s\n"+
			"修法：给 handler 补 swag 注解并重新生成契约；若它本就不该进契约，在 allowNotInContract 里写清理由。",
			len(undocumented), strings.Join(undocumented, "\n  "))
	}

	// 白名单自身的防腐：登记了却已经不存在（或已经进了契约）的条目要删掉，
	// 否则这张表会慢慢变成"没人敢动的历史垃圾场"。
	var stale []string
	for op := range allowNotInContract {
		if inContract[op] || !registered[op] {
			stale = append(stale, op)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("白名单有 %d 条已失效（该路由不存在，或已经进了契约），请删除：\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}

var (
	// gin 用 :id，OpenAPI 用 {id}；统一成后者再比。
	ginParam = regexp.MustCompile(`[:*]([A-Za-z_][A-Za-z0-9_]*)`)
)

// normalizeRoute 把 gin 的路由条目映射到契约的写法：
// 去掉 /api 前缀（契约里的路径不含它），参数占位符统一成 {name}。
func normalizeRoute(method, path string) string {
	p := strings.TrimPrefix(path, "/api")
	return strings.ToUpper(method) + " " + ginParam.ReplaceAllString(p, "{$1}")
}

// readContractOps 读出契约里所有 "METHOD /path"。
func readContractOps() ([]string, error) {
	// 测试的工作目录是包目录：backend/internal/platform/http/router，
	// 向上四级正好是 backend/。
	path := filepath.Join("..", "..", "..", "..", "docs", "swagger.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	methods := map[string]bool{
		"get": true, "post": true, "put": true, "delete": true, "patch": true,
		"head": true, "options": true, "trace": true,
	}
	ops := make([]string, 0, len(doc.Paths))
	for p, item := range doc.Paths {
		for m := range item {
			if methods[m] {
				ops = append(ops, strings.ToUpper(m)+" "+p)
			}
		}
	}
	sort.Strings(ops)
	return ops, nil
}
