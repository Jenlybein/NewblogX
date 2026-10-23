package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func TestCorsMiddlewarePreflightAndActualRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CorsMiddleware())
	r.POST("/api/users/login", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	origin := "http://localhost:3001"

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/users/login", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "token, content-type")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("预检请求状态码异常: %d", w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("预检请求未返回允许来源: %q", got)
		}
		if got := strings.ToLower(w.Header().Get("Access-Control-Allow-Headers")); !strings.Contains(got, "token") {
			t.Fatalf("预检请求未放行 token 请求头: %q", got)
		}
	}

	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/users/login", nil)
		req.Header.Set("Origin", origin)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("实际请求状态码异常: %d", w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("实际请求未返回允许来源: %q", got)
		}
	}
}

func TestCorsMiddlewareRejectsUnknownOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CorsMiddleware())
	r.POST("/api/users/login", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/users/login", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("未知来源预检请求应被拒绝: %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("未知来源不应返回 CORS 允许头: %q", got)
	}
}

func TestOriginAllowlistFromEnv(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "https://front.example.com, http://localhost:4000/")

	if !middleware.IsAllowedOrigin("https://front.example.com") {
		t.Fatal("环境变量中的正式来源应被允许")
	}
	if !middleware.IsAllowedOrigin("http://localhost:4000") {
		t.Fatal("环境变量中的本地来源应被允许")
	}
	if middleware.IsAllowedOrigin("http://localhost:3000") {
		t.Fatal("设置环境变量后应替换默认白名单，而不是继续合并默认值")
	}
}

func TestOriginAllowlistRejectsUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{"*", "null", "file://local/index.html", "https://blog.gentlybeing.cn/path"} {
		if middleware.IsAllowedOrigin(origin) {
			t.Fatalf("不安全或非法来源应被拒绝: %q", origin)
		}
	}
}
