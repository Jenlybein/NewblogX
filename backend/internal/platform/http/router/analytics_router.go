package router

import (
	analyticshttp "myblogx/internal/analytics/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// analyticsHandler 由 app.WireAnalytics 在启动时注入。
//
// 站点统计、日志查询、用户登录日志共用一个处理器。
var analyticsHandler *analyticshttp.Handler

// SetAnalyticsHandler 注入统计处理器。
func SetAnalyticsHandler(h *analyticshttp.Handler) { analyticsHandler = h }

// mustAnalyticsHandler 取处理器，未注入直接 panic（与 article 一致）。
func mustAnalyticsHandler() *analyticshttp.Handler {
	if analyticsHandler == nil {
		panic("router: 统计处理器未注入（应在 app.WireAnalytics 中调用 SetAnalyticsHandler）")
	}
	return analyticsHandler
}

// DataRouter 注册站点统计路由。
func DataRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustAnalyticsHandler()

	group := r.Group("data", runtimeMw.AuthMiddleware)
	adminGroup := group.Group("", mw.AdminMiddleware)

	adminGroup.GET("sum", app.SumView)
	adminGroup.GET("growth", mw.BindQuery[analyticshttp.GrowthDataRequest], app.GrowthDataView)
	adminGroup.GET("article-year", app.ArticleYearDataView)
}

// LogRouter 注册日志查询路由。
func LogRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustAnalyticsHandler()

	group := r.Group("logs")
	group.Use(runtimeMw.AuthMiddleware, mw.AdminMiddleware)

	group.GET("runtime", mw.BindQuery[analyticshttp.RuntimeLogListRequest], app.RuntimeLogListView)
	group.GET("runtime/:id", mw.BindUri[analyticshttp.IDRequest], app.RuntimeLogDetailView)

	group.GET("login", mw.BindQuery[analyticshttp.LoginLogListRequest], app.LoginLogListView)
	group.GET("login/:id", mw.BindUri[analyticshttp.IDRequest], app.LoginLogDetailView)

	group.GET("action", mw.BindQuery[analyticshttp.ActionAuditListRequest], app.ActionAuditListView)
	group.GET("action/:id", mw.BindUri[analyticshttp.IDRequest], app.ActionAuditDetailView)

	group.GET("cdc", mw.BindQuery[analyticshttp.CdcEventListRequest], app.CdcEventListView)
	group.GET("cdc/:id", mw.BindUri[analyticshttp.IDRequest], app.CdcEventDetailView)

	group.GET("replay", mw.BindQuery[analyticshttp.ReplayEventListRequest], app.ReplayEventListView)
	group.GET("replay/:id", mw.BindUri[analyticshttp.IDRequest], app.ReplayEventDetailView)

	group.GET("chain", mw.BindQuery[analyticshttp.ChainQueryRequest], app.ChainQueryView)

	// 死信（DLQ）：看得见 + 补得上。重投是写操作，天然落在这个 admin 分组里。
	group.GET("dead-letter", mw.BindQuery[analyticshttp.DeadLetterListRequest], app.DeadLetterListView)
	group.POST("dead-letter/replay", mw.BindJson[analyticshttp.DeadLetterReplayRequest], app.DeadLetterReplayView)
}
