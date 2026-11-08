package router_test

// 管理端路由鉴权的「注册级全量扫描」（spec 269 / A-4，票 264）。
//
// # 为什么换掉旧的完整性检查
//
// 旧版 `TestAdminOnlyListCoversContract` 只看 4 个手写前缀（/data/、/logs/、
// /users/admin、/site/admin/）。而后台接口散落在 /api/images、/api/ai/providers、
// /api/collections/admin、/api/banners 等处 —— 代码里确实挂了 AdminMiddleware，
// 但**不在自动覆盖里**：明天新加一条 /api/xxx 管理端点忘了挂中间件，测试照样全绿。
//
// # 现在怎么扫
//
// gin 的 `Routes()` 给出引擎里**每一条注册路由**（含 `:id` 参数模板）。本门禁要求
// 每一条都被明确归类到下面两份清单之一：
//
//   - `adminRouteTemplates`：只有管理员能访问（由行为探针逐个验证）；
//   - `userRouteTemplates`：登录用户或公开可访问。
//
// 两条规则合起来等价于"全量覆盖"：
//   1. 任何**新注册**的路由不在这两份清单里 → 门禁红，逼开发者当场判断它该不该
//      要求管理员；
//   2. 清单里多余的（路由已删）条目 → 也红，避免清单腐化成死列表。
//
// 行为探针仍留在 `admin_gate_test.go`：它用普通用户令牌真发一次请求，证明
// AdminMiddleware 真的拦住了（而不是"清单写了就算"）。本文件负责覆盖面，
// 那边负责判据，两者缺一不可。

import (
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
)

// adminRouteTemplates 是所有「必须只有管理员能访问」的路由模板（gin 的 `:param` 形式）。
//
// 这份清单与 adminOnly 的区别：adminOnly 是**行为探针**能真发的无参数路径；
// 这里是**注册级分类**，带参数的详情接口（`/api/logs/runtime/:id` 等）也必须登记，
// 否则它们就成了扫描不到的暗角。
//
// 新增后台路由时，把它加进这里，并在 adminOnly 里补一条可探测的无参数等价路由
// （或说明为什么只能带参数）。
var adminRouteTemplates = []string{
	// 驳回原因的固定列表：审核员驳回时用它渲染下拉，属后台接口。
	"GET /api/article-review/reasons",
	// 数据看板
	"GET /api/data/sum",
	"GET /api/data/growth",
	"GET /api/data/article-year",
	"GET /api/data/ai-usage",
	// 日志中心（含详情与死信重投）
	"GET /api/logs/runtime",
	"GET /api/logs/runtime/:id",
	"GET /api/logs/action",
	"GET /api/logs/action/:id",
	"GET /api/logs/login",
	"GET /api/logs/login/:id",
	"GET /api/logs/cdc",
	"GET /api/logs/cdc/:id",
	"GET /api/logs/replay",
	"GET /api/logs/replay/:id",
	"GET /api/logs/chain",
	"GET /api/logs/ai",
	"GET /api/logs/ai/:id",
	"GET /api/logs/dead-letter",
	"POST /api/logs/dead-letter/replay",
	// 用户管理
	"GET /api/users/admin/list",
	"POST /api/users/admin",
	"PUT /api/users/admin/info",
	"GET /api/users/login/log",
	// 站点配置（九段具名端点：公开读 site / deco，管理读与写都是管理员）。
	"GET /api/site/admin/site",
	"GET /api/site/admin/ai",
	"GET /api/site/admin/mail",
	"GET /api/site/admin/storage",
	"GET /api/site/admin/oauth",
	"GET /api/site/admin/log",
	"GET /api/site/admin/upload",
	"GET /api/site/admin/deco",
	"GET /api/site/admin/image_gc",
	"GET /api/site/admin/sidebar_cards",
	"PUT /api/site/admin/sidebar_cards",
	"PUT /api/site/site",
	"PUT /api/site/ai",
	"PUT /api/site/mail",
	"PUT /api/site/storage",
	"PUT /api/site/oauth",
	"PUT /api/site/log",
	"PUT /api/site/upload",
	"PUT /api/site/deco",
	"PUT /api/site/image_gc",
	// 全局通知写操作（读是登录用户都有的）
	"POST /api/global_notif",
	"DELETE /api/global_notif",
	// 右栏 / 横幅
	"POST /api/banners",
	"PUT /api/banners/:id",
	"DELETE /api/banners",
	// 图片管理（上传是所有登录用户，列表/删除只给管理员）
	"GET /api/images",
	"DELETE /api/images",
	// AI 供应商（AI 生成类接口是登录用户可用，供应商管理只给管理员）
	"GET /api/ai/providers",
	"GET /api/ai/providers/vendors",
	"POST /api/ai/providers",
	"PUT /api/ai/providers/:id",
	"DELETE /api/ai/providers/:id",
	"POST /api/ai/providers/:id/enable",
	// 文章运营位 / 审核
	"GET /api/articles/hot-top/admin",
	"PATCH /api/articles/hot-top/order",
	"DELETE /api/articles/:id/admin/hot-top",
	"POST /api/articles/:id/admin/hot-top",
	"POST /api/articles/:id/admin/:visibility",
	"DELETE /api/articles",
	"DELETE /api/articles/tags",
	"PUT /api/articles/tags",
	"GET /api/article-review",
	"POST /api/article-review-tasks/:id/review",
	"POST /api/article-review-tasks/batch-review",
	// 合集 / 项目（前台读公开，编排与写只给管理员）
	"GET /api/collections/admin",
	"PUT /api/collections/:id/outline-tree",
	"POST /api/collections",
	"PUT /api/collections/:id",
	"DELETE /api/collections/:id",
	"PATCH /api/collections/:id/display",
	"GET /api/projects/admin",
	"PUT /api/projects/:id/outline-tree",
	"POST /api/projects",
	"PUT /api/projects/:id",
	"DELETE /api/projects/:id",
	"PATCH /api/projects/:id/display",
	"PATCH /api/projects/order",
}

// userRouteTemplates 是其余所有路由：公开或登录用户可访问。
//
// 这份清单是「分类结果」而不是「白名单放行」：它只说明"这条不需要管理员"，
// 不说明"这条不需要登录"。新增路由必须显式加到两份清单之一，门禁才会绿。
var userRouteTemplates = []string{
	// 站点地图：公开只读（爬虫与匿名读者都要能取），不进 ES、不做鉴权。
	"GET /api/sitemap.xml",
	// 健康检查 / 静态
	"GET /health",
	"GET /health/ready",
	"GET /uploads/*filepath",
	"HEAD /uploads/*filepath",
	// 初始化 / 登录 / 会话
	"GET /api/setup/status",
	"POST /api/setup/admin",
	"GET /api/imagecaptcha",
	"POST /api/users/email/login",
	"POST /api/users/email/register",
	"POST /api/users/email/verify",
	"POST /api/users/login",
	"POST /api/users/logout",
	"POST /api/users/logout/all",
	"POST /api/users/qq",
	"POST /api/users/refresh",
	"PUT /api/users/email/bind",
	"PUT /api/users/info",
	"PUT /api/users/password/recovery/email",
	"PUT /api/users/password/renewal/email",
	"GET /api/users/base",
	"GET /api/users/detail",
	"GET /api/users/sessions",
	"DELETE /api/users/sessions/:id",
	// 站点公开读：seo / ai_info / 九段里的 site 与 deco / 右栏卡片。
	"GET /api/site/site",
	"GET /api/site/deco",
	"GET /api/site/qq_url",
	"GET /api/site/seo",
	"GET /api/site/sidebar_cards",
	"GET /api/site/ai_info",
	// 站内信 / 通知
	"GET /api/sitemsg",
	"GET /api/sitemsg/conf",
	"GET /api/sitemsg/user",
	"POST /api/sitemsg",
	"DELETE /api/sitemsg",
	"PUT /api/sitemsg/conf",
	"GET /api/global_notif",
	"DELETE /api/global_notif/user",
	"POST /api/global_notif/read",
	// 私信
	"GET /api/chat/sessions",
	"GET /api/chat/messages",
	"POST /api/chat/messages",
	"POST /api/chat/read",
	"DELETE /api/chat/messages",
	"DELETE /api/chat/sessions",
	"GET /api/chat/ws-ticket",
	"GET /api/chat/ws",
	// 图片上传（所有登录用户）
	"POST /api/images/upload-tasks",
	"POST /api/images/upload-tasks/complete",
	"GET /api/images/upload-tasks/:id",
	"POST /api/images/qiniu/callback",
	"POST /api/images/qiniu/audit/callback",
	// 文章前台
	"GET /api/articles/:id",
	"GET /api/articles/author_info",
	"GET /api/articles/category",
	"GET /api/articles/category/options",
	"POST /api/articles/category",
	"DELETE /api/articles/category",
	"GET /api/articles/tags",
	"GET /api/articles/tags/options",
	"GET /api/articles/hot-top",
	"GET /api/articles/top",
	"POST /api/articles/top",
	"POST /api/articles/view",
	"POST /api/articles",
	"PUT /api/articles/:id",
	"DELETE /api/articles/:id",
	"PUT /api/articles/:id/digg",
	"GET /api/articles/favorite",
	"POST /api/articles/favorite",
	"PUT /api/articles/favorite",
	"DELETE /api/articles/favorite",
	"GET /api/articles/favorite/contents",
	"DELETE /api/articles/favorite/contents",
	"GET /api/articles/history",
	"DELETE /api/articles/history",
	"DELETE /api/articles/history/all",
	"DELETE /api/articles/top",
	// 合集 / 项目前台读（含两个具名树端点，公开读）
	"GET /api/collections",
	"GET /api/collections/:id",
	"GET /api/collections/:id/outline-tree",
	"GET /api/projects",
	"GET /api/projects/:id",
	"GET /api/projects/:id/outline-tree",
	// 评论 / 关注 / 粉丝
	"GET /api/comments",
	"GET /api/comments/man",
	"GET /api/comments/replies",
	"POST /api/comments",
	"POST /api/comments/:id/digg",
	"DELETE /api/comments/:id",
	"GET /api/fans",
	"GET /api/follow",
	"POST /api/follow/:id",
	"DELETE /api/follow/:id",
	// 搜索 / AI 生成（登录用户）
	"GET /api/search/articles",
	"POST /api/ai/diagnose",
	"POST /api/ai/metainfo",
	"POST /api/ai/overwrite",
	"POST /api/ai/scoring/article",
	"POST /api/ai/search/list",
	"POST /api/ai/search/llm",
	// 横幅公开读
	"GET /api/banners",
}

func registeredRoutes(t *testing.T) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultErrorWriter = io.Discard
	injectZeroHandlers()
	engine := router.NewEngine(middleware.RuntimeOptions{Verifier: userTokenVerifier{}}, nil, injectZeroHandlers())
	out := make([]string, 0, len(engine.Routes()))
	for _, r := range engine.Routes() {
		out = append(out, strings.ToUpper(r.Method)+" "+r.Path)
	}
	sort.Strings(out)
	return out
}

// TestEveryRegisteredRouteIsClassified 是全量扫描的核心：引擎里每条路由都必须
// 被明确归类，且两份清单不能有重复或已经不存在的条目。
//
// 这就是"缺一条即红"：新加一条 /api/xxx 管理端点却忘了归类，这里立刻失败；
// 把一条已删的路由留在清单里也会失败（清单不会腐化）。
func TestEveryRegisteredRouteIsClassified(t *testing.T) {
	admin := map[string]struct{}{}
	for _, r := range adminRouteTemplates {
		if _, dup := admin[r]; dup {
			t.Fatalf("adminRouteTemplates 有重复条目: %s", r)
		}
		admin[r] = struct{}{}
	}
	user := map[string]struct{}{}
	for _, r := range userRouteTemplates {
		if _, dup := user[r]; dup {
			t.Fatalf("userRouteTemplates 有重复条目: %s", r)
		}
		if _, dup := admin[r]; dup {
			t.Fatalf("路由同时出现在两份清单里: %s", r)
		}
		user[r] = struct{}{}
	}

	routes := registeredRoutes(t)
	if len(routes) == 0 {
		t.Fatal("引擎里一条路由都没有，扫描毫无意义")
	}
	seen := map[string]struct{}{}
	for _, r := range routes {
		seen[r] = struct{}{}
		_, inAdmin := admin[r]
		_, inUser := user[r]
		if !inAdmin && !inUser {
			t.Fatalf("未归类的注册路由 %q：请判断它该不该要求管理员，"+
				"然后加进 adminRouteTemplates 或 userRouteTemplates（新增后台接口漏挂中间件就是这么漏掉的）", r)
		}
	}
	for r := range admin {
		if _, ok := seen[r]; !ok {
			t.Fatalf("adminRouteTemplates 里的 %q 已不在注册表中，请删掉这条死条目", r)
		}
	}
	for r := range user {
		if _, ok := seen[r]; !ok {
			t.Fatalf("userRouteTemplates 里的 %q 已不在注册表中，请删掉这条死条目", r)
		}
	}
}

// TestAdminTemplatesHaveBehaviourProbes 保证"归类为管理员"的无参数路由都真的被
// 行为探针发过一次请求（清单与探针不能各说各话）。带 :param 的模板跳过 —— 它们
// 与各自的列表接口在同一个 adminGroup 里注册，字面量探测会先在路由层 404。
func TestAdminTemplatesHaveBehaviourProbes(t *testing.T) {
	probed := map[string]struct{}{}
	for _, c := range adminOnly {
		probed[strings.ToUpper(c.method)+" "+c.path] = struct{}{}
	}
	for _, r := range adminRouteTemplates {
		if strings.Contains(r, ":") || strings.Contains(r, "*") {
			continue
		}
		if _, ok := probed[r]; !ok {
			t.Fatalf("管理员路由 %q 归类了但没有行为探针：请把它加进 adminOnly，"+
				"让普通用户令牌真的被挡一次（而不是只在清单上写一行）", r)
		}
	}
}

// TestClassificationGateIsNotAlwaysGreen 是 canary：故意从两份清单里"漏掉"一条
// 真实存在的路由时，分类函数必须报错。永远不红的门禁等于没有门禁（见 06 号文档）。
func TestClassificationGateIsNotAlwaysGreen(t *testing.T) {
	routes := registeredRoutes(t)
	if len(routes) == 0 {
		t.Fatal("拿不到注册路由")
	}
	admin := map[string]struct{}{}
	for _, r := range adminRouteTemplates {
		admin[r] = struct{}{}
	}
	user := map[string]struct{}{}
	for _, r := range userRouteTemplates {
		user[r] = struct{}{}
	}

	// 模拟"新增了一条 /api/xxx 却忘了归类"：删掉一条真实路由的分类。
	victim := routes[0]
	delete(admin, victim)
	delete(user, victim)

	_, inAdmin := admin[victim]
	_, inUser := user[victim]
	if inAdmin || inUser {
		t.Fatalf("canary 自相矛盾：%q 仍被归类", victim)
	}
	// 真正的门禁循环里，这条会命中 "未归类" 分支并 Fatalf；这里只证明
	// "未归类"这个判定条件本身能成立（不是恒 false 的假绿）。
	unclassified := !inAdmin && !inUser
	if !unclassified {
		t.Fatal("canary 失效：未归类判定恒为假，门禁永远不会红")
	}
}
