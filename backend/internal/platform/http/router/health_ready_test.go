package router_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"myblogx/internal/platform/health"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"

	"github.com/gin-gonic/gin"
)

type readyPayload struct {
	Healthy    bool               `json:"healthy"`
	Components []health.Component `json:"components"`
}

func fetchReady(t *testing.T, engine *gin.Engine) (int, readyPayload) {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	engine.ServeHTTP(rec, req)

	var payload readyPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("就绪响应不是合法 JSON: %v，原文=%s", err, rec.Body.String())
	}
	return rec.Code, payload
}

// /health/ready 是这次事故的产物：river 死了但进程还活着的时候，
// 必须有一处能返回 503，否则"发布成功但文章永远搜不到"只能靠人肉翻日志发现。
func TestHealthReadyTurns503WhenComponentFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := health.New()
	engine := router.NewEngine(middleware.RuntimeOptions{}, registry)

	// 组件都在正常/未启用状态：就绪。
	registry.Set("cdc_river", health.StateRunning, nil)
	registry.Set("image_ref_river", health.StateDisabled, nil)

	code, payload := fetchReady(t, engine)
	if code != http.StatusOK {
		t.Fatalf("组件正常时应为 200，实际 %d", code)
	}
	if !payload.Healthy {
		t.Fatal("组件正常时 healthy 应为 true")
	}
	if len(payload.Components) != 2 {
		t.Fatalf("应返回 2 个组件，实际 %d", len(payload.Components))
	}

	// 复刻事故现场：canal 起不来，river 记成 failed。
	registry.Set("cdc_river", health.StateFailed, errors.New("启动 Canal 同步失败: dump exit 2"))

	code, payload = fetchReady(t, engine)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("组件失败时应为 503，实际 %d", code)
	}
	if payload.Healthy {
		t.Fatal("组件失败时 healthy 应为 false")
	}

	// 必须能看出"是哪个组件、为什么挂"，否则这个端点只是把红灯换个地方亮。
	var failed health.Component
	for _, c := range payload.Components {
		if c.Name == "cdc_river" {
			failed = c
		}
	}
	if failed.State != health.StateFailed {
		t.Fatalf("cdc_river 状态应为 failed，实际 %q", failed.State)
	}
	if failed.Error == "" {
		t.Fatal("失败原因不能为空")
	}
}

// 只跑 HTTP 的角色（-role=api）没有后台组件，registry 为 nil。
// 此时就绪检查必须仍然可用，且 components 是空数组而不是 null ——
// 探针按数组解析，null 会让它的类型判断炸掉。
func TestHealthReadyWithoutRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := router.NewEngine(middleware.RuntimeOptions{}, nil)

	code, payload := fetchReady(t, engine)
	if code != http.StatusOK {
		t.Fatalf("没有后台组件时应为 200，实际 %d", code)
	}
	if payload.Components == nil {
		t.Fatal("components 必须是空数组，不能是 null")
	}
	if len(payload.Components) != 0 {
		t.Fatalf("不应有组件，实际 %d", len(payload.Components))
	}
}
