package router

import (
	aihttp "myblogx/internal/ai/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// aiHandler 由 app.WireAI 在启动时注入。
var aiHandler *aihttp.Handler

// SetAIHandler 注入 AI 处理器。
func SetAIHandler(h *aihttp.Handler) { aiHandler = h }

// AIRouter 注册 AI 路由。
func AIRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if aiHandler == nil {
		panic("router: AI 处理器未注入（应在 app.WireAI 中调用 SetAIHandler）")
	}
	app := aiHandler

	group := r.Group("ai")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	group.POST("scoring/article", runtimeMw.OptionalAuthMiddleware, mw.BindJson[aihttp.ScoringRequest], app.Scoring)
	authGroup.POST("metainfo", mw.BindJson[aihttp.BaseRequest], app.MetaInfo)
	authGroup.POST("overwrite", mw.BindJson[aihttp.OverwriteRequest], app.Overwrite)
	authGroup.POST("diagnose", mw.BindJson[aihttp.DiagnoseRequest], app.Diagnose)
	authGroup.POST("search/list", mw.BindJson[aihttp.BaseRequest], app.SearchList)
	authGroup.POST("search/llm", mw.BindJson[aihttp.BaseRequest], app.SearchLLM)

	// 模型配置列表（管理员）：静态路径必须先于 :id 注册，否则会被通配吃掉。
	adminGroup.GET("providers/vendors", app.ProviderVendors)
	adminGroup.GET("providers", app.ProviderList)
	adminGroup.POST("providers", mw.CaptureLog(mw.ReqBody), app.ProviderCreate)
	adminGroup.PUT("providers/:id", mw.CaptureLog(mw.ReqBody), mw.BindUri[aihttp.ProviderIDRequest], app.ProviderUpdate)
	adminGroup.POST("providers/:id/enable", mw.CaptureLog(mw.ReqBody), mw.BindUri[aihttp.ProviderIDRequest], app.ProviderEnable)
	adminGroup.DELETE("providers/:id", mw.CaptureLog(mw.ReqBody), mw.BindUri[aihttp.ProviderIDRequest], app.ProviderRemove)
}
