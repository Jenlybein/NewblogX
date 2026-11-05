package router_test

// #277 收缩：旧共享树端点与旧共用树表从对外形状里退役。
//
// 这条守卫跨的是「契约 ↔ 路由表」这一条行为接缝：退役清单里的每个 operation
// 既不能出现在路由表，也不能出现在契约里；保留清单里的具名树端点必须在两边
// 都在（否则整份路由表清空也能全绿，那是另一种假绿）。
//
// 与 retired_flat_include_test.go 的 #225 守卫同形，但清单是 #277 这一批的：
// 旧共享 operation 全部来自 #274 expand 期的票内过渡，不是长期兼容层。

import (
	"testing"

	"github.com/gin-gonic/gin"

	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
)

// retiredSharedOutlineOps 是 #277 收缩掉的旧共享 operation。
var retiredSharedOutlineOps = []string{
	"GET /collections/{id}/outline/admin",
	"PUT /collections/{id}/outline",
	"GET /projects/{id}/outline/admin",
	"PUT /projects/{id}/outline",
	"GET /articles/{id}/outline-context",
}

// keptNamedOutlineOps 是收缩后**唯一**对外的树端点形状。
var keptNamedOutlineOps = []string{
	"GET /collections/{id}/outline-tree",
	"PUT /collections/{id}/outline-tree",
	"GET /projects/{id}/outline-tree",
	"PUT /projects/{id}/outline-tree",
}

func TestRetiredSharedOutlineOpsAreGone(t *testing.T) {
	gin.SetMode(gin.TestMode)

	injectZeroHandlers()
	engine := router.NewEngine(mw.RuntimeOptions{}, nil)

	registered := make(map[string]bool)
	for _, rt := range engine.Routes() {
		registered[normalizeRoute(rt.Method, rt.Path)] = true
	}
	contracted, err := readContractOps()
	if err != nil {
		t.Fatalf("读取契约失败: %v", err)
	}
	inContract := make(map[string]bool, len(contracted))
	for _, op := range contracted {
		inContract[op] = true
	}

	for _, op := range retiredSharedOutlineOps {
		if registered[op] {
			t.Errorf("路由表仍注册了旧共享端点 %s：expand 期的并存应在本票收缩掉", op)
		}
		if inContract[op] {
			t.Errorf("契约里仍有旧共享端点 %s：删掉 handler 后必须重新生成三份契约产物", op)
		}
	}
	for _, op := range keptNamedOutlineOps {
		if !registered[op] {
			t.Errorf("具名树端点 %s 不在路由表上：收缩时误删了唯一对外形状", op)
		}
		if !inContract[op] {
			t.Errorf("具名树端点 %s 不在契约里：注解被误删或契约忘了重新生成", op)
		}
	}
}
