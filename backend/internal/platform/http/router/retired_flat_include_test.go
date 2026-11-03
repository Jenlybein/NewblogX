package router_test

// 旧扁平收录 / 排序接口的退役守卫（#225）。
//
// # 为什么需要它
//
// #218 之后两端前端不再调用这些接口，但它们仍留在路由表与契约里，写的正是
// 已被用户要求**删表**的两张旧扁平收录中间表。表一旦在部署时 DROP，
// 这些接口就会从"没人调用"变成"一调就 500"。
//
// 删代码这件事本身没有编译器保障：注解与路由是两份独立事实（见
// contract_routes_test.go 的教训），删掉 handler 却忘了删路由会留下一条
// 直指空方法的 404，删掉路由却忘了重新生成契约会让前端继续以为它在。
//
// 所以这里把"退役"钉成双向规则：
//  1. 退役清单里的每个 operation 既不能出现在路由表，也不能出现在契约里；
//  2. 保留清单里的 operation 必须在两边都在（尤其是 `PATCH /projects/order`：
//     它写的是 `project_models.sort_order`，是现行功能，不属于退役范围）。
//
// 第 2 条是这条守卫的反面证明：如果它只断言"不存在"，把整个路由表清空也能过。

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
)

// retiredFlatIncludeOps 是 #225 删除的旧扁平收录 / 排序接口。
//
// 它们全部只服务两张已冻结的旧中间表；编排入口自 #218 起统一是目录树的整树提交。
// `PATCH /projects/order` 刻意**不在**这里（见 keptFlatSuccessorOps）。
var retiredFlatIncludeOps = []string{
	// 合集：收录行查询 / 单篇收录 / 批量收录 / 单条移除 / 合集内排序 / 失效清理 /
	// 合集之间排序（写 collection_models.sort_order，但已无调用方）。
	"POST /collections/{id}/articles",
	"POST /collections/{id}/articles/batch",
	"GET /collections/{id}/articles/admin",
	"DELETE /collections/{id}/articles",
	"PATCH /collections/{id}/articles/order",
	"DELETE /collections/{id}/admin/stale",
	"PATCH /collections/order",
	// 项目：与合集逐条同构，唯一例外是下面的 `PATCH /projects/order`。
	"POST /projects/{id}/articles",
	"POST /projects/{id}/articles/batch",
	"GET /projects/{id}/articles/admin",
	"DELETE /projects/{id}/articles",
	"PATCH /projects/{id}/articles/order",
	"DELETE /projects/{id}/admin/stale",
}

// keptFlatSuccessorOps 是退役后**必须仍然存在**的接口。
//
// 前三组是这次删改的边界：目录树、详情、后台卡片、展示开关，以及项目之间排序。
// 少任何一条都说明"删旧接口"顺手删掉了现行功能。
var keptFlatSuccessorOps = []string{
	// 项目之间排序：写 project_models.sort_order，仍是现行功能。
	"PATCH /projects/order",
	// 目录树读写（合集与项目）。
	"GET /collections/{id}/outline/admin",
	"PUT /collections/{id}/outline",
	"GET /projects/{id}/outline/admin",
	"PUT /projects/{id}/outline",
	// 详情（读者侧）。
	"GET /collections/{id}",
	"GET /projects/{id}",
	"GET /articles/{id}/outline-context",
	// 后台卡片与增删改、展示开关。
	"GET /collections/admin",
	"POST /collections",
	"PUT /collections/{id}",
	"DELETE /collections/{id}",
	"PATCH /collections/{id}/display",
	"GET /projects/admin",
	"POST /projects",
	"PUT /projects/{id}",
	"DELETE /projects/{id}",
	"PATCH /projects/{id}/display",
	// 读者侧列表。
	"GET /collections",
	"GET /projects",
}

// TestRetiredFlatIncludeOpsAreGone 是这条守卫的主体。
func TestRetiredFlatIncludeOpsAreGone(t *testing.T) {
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

	for _, op := range retiredFlatIncludeOps {
		if registered[op] {
			t.Errorf("路由表仍注册了退役接口 %s：它写的是已冻结的旧扁平收录表，"+
				"删表之后一调就是 500", op)
		}
		if inContract[op] {
			t.Errorf("契约里仍有退役接口 %s：删掉 handler 后必须重新生成 swagger.json "+
				"（swag init）与两端 openapi.d.ts", op)
		}
	}

	for _, op := range keptFlatSuccessorOps {
		if !registered[op] {
			t.Errorf("保留接口 %s 不在路由表上：删旧接口时误删了现行功能", op)
		}
		if !inContract[op] {
			t.Errorf("保留接口 %s 不在契约里：注解被误删或契约忘了重新生成", op)
		}
	}
}

// TestRetiredFlatIncludeListHasNoOverlap 防止两份清单打架。
//
// 同一条 operation 同时出现在"必须消失"和"必须存在"里时，上面那条用例
// 无论怎么实现都会红一半——那是清单写错了，不是代码错了。
func TestRetiredFlatIncludeListHasNoOverlap(t *testing.T) {
	kept := make(map[string]bool, len(keptFlatSuccessorOps))
	for _, op := range keptFlatSuccessorOps {
		kept[op] = true
	}
	for _, op := range retiredFlatIncludeOps {
		if kept[op] {
			t.Errorf("%s 同时出现在退役清单与保留清单里", op)
		}
	}
}

// TestRetiredFlatIncludeGuardCatchesPhantom 证明这条守卫**真的会红**。
//
// 拿一条真实存在且保留的 operation 喂给同一套判据：它必须被判成"仍然存在"。
// 否则上面那条用例只能证明 `registered` 里查不到任何东西（一张空表也能全绿）。
func TestRetiredFlatIncludeGuardCatchesPhantom(t *testing.T) {
	gin.SetMode(gin.TestMode)

	injectZeroHandlers()
	engine := router.NewEngine(mw.RuntimeOptions{}, nil)

	registered := make(map[string]bool)
	for _, rt := range engine.Routes() {
		registered[normalizeRoute(rt.Method, rt.Path)] = true
	}

	// 保留清单里的任意一条都必须在路由表上——这就是"查得到"的方向。
	probe := keptFlatSuccessorOps[0]
	if !registered[probe] {
		t.Fatalf("对照接口 %s 应当存在（守卫的阳性方向失效了）", probe)
	}
	if !strings.HasPrefix(probe, "PATCH /projects/order") {
		t.Fatalf("对照接口应当是项目之间排序，实际 %s", probe)
	}
}
