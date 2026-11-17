package router_test

// 关键词风险端点的鉴权（ADR-0032 决策 7 / 票 396）。
//
// 这里只证明一件事：**匿名**请求到不了响应体。普通用户被 AdminMiddleware 挡住那条
// 由 admin_gate_test.go 的注册级全量扫描覆盖（本端点已登记在 adminOnly 与
// adminRouteTemplates 里），两处分工不同：
//   - 这里：未登录 → 鉴权失败（专属码 1003），不是 404、也不是"返回空列表"；
//   - 那边：登录了但不是管理员 → 权限错误。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/router"
)

// TestContentFilterHitsRejectsAnonymous 匿名读命中记录必须被鉴权层挡下。
func TestContentFilterHitsRejectsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gin.DefaultErrorWriter = io.Discard

	engine := router.NewEngine(mw.RuntimeOptions{Verifier: userTokenVerifier{}}, nil, injectZeroHandlers())

	// 刻意不带 Authorization：模拟未登录的浏览器直接打这个 URL。
	req := httptest.NewRequest("GET", "/api/content-filter/hits", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data any    `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON（status=%d body=%q）：这条路由大概没挂 AuthMiddleware",
			rec.Code, rec.Body.String())
	}
	// 鉴权失败用专属码 1003：前端据此判断"要不要刷新令牌"，不必匹配中文文案。
	if rec.Code != http.StatusOK || body.Code != int(res.FailAuthCode) {
		t.Fatalf("匿名应拿到鉴权失败（HTTP 200 / code=%d），实际 status=%d code=%d msg=%q",
			int(res.FailAuthCode), rec.Code, body.Code, body.Msg)
	}
	if strings.Contains(rec.Body.String(), "list") {
		t.Fatal("匿名响应不该出现任何命中记录字段（那等于把风险页数据泄给未登录用户）")
	}
}
