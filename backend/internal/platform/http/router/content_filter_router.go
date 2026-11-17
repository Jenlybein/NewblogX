package router

import (
	filterhttp "myblogx/internal/filter/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// filterHandler 由 app.WireFilter 在启动时注入。
var filterHandler *filterhttp.Handler

// SetFilterHandler 注入关键词风险处理器。
func SetFilterHandler(h *filterhttp.Handler) { filterHandler = h }

// mustFilterHandler 取处理器，未注入直接 panic（与其它模块一致：
// 装配漏了就该在第一次请求前炸掉，而不是让某个后台页面静默 404）。
func mustFilterHandler() *filterhttp.Handler {
	if filterHandler == nil {
		panic("router: 内容过滤处理器未注入（应在 app.WireFilter 中调用 SetFilterHandler）")
	}
	return filterHandler
}

// ContentFilterRouter 注册关键词风险路由（ADR-0032 决策 7）。
//
// 整段只给管理员：看得到的是"谁发了什么命中词"，本身就是敏感信息；处置动作
// 还会软删内容 / 重置资料，读者站与任何登录用户都拿不到。
//
// `hits/handle-batch`（静态段）与 `hits/:id/handle`（参数段）是同一位置上的
// 两个子节点：gin 会优先匹配静态段，所以批量端点不会被 `:id = "handle-batch"`
// 吃掉。这里仍把批量写在前面，纯粹是让"两个处置端点"读起来成对。
func ContentFilterRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustFilterHandler()

	group := r.Group("content-filter")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	adminGroup.GET("hits", app.ListHits)
	adminGroup.POST("hits/handle-batch", app.HandleBatchHits)
	adminGroup.POST("hits/:id/handle", app.HandleHit)
	// 词云统计（ADR-0032 决策 9）：字号 = 最近 90 天出现处数。
	adminGroup.GET("word-stats", app.WordStats)
}
