package router

import (
	moderationhttp "myblogx/internal/moderation/adapter/http"
	"myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// moderationHandler 由装配层在启动时注入。
var moderationHandler *moderationhttp.Handler

// SetModerationHandler 注入违规计分处理器。
func SetModerationHandler(h *moderationhttp.Handler) { moderationHandler = h }

// ModerationRouter 注册违规计分与申诉路由。
//
// 两条面：`/moderation/me*` 是登录用户看自己的记录并申诉；
// `/moderation/appeals*` 是管理员处置。
func ModerationRouter(r *gin.RouterGroup, runtimeMw middleware.Runtime) {
	if moderationHandler == nil {
		// 未装配（测试或最小装配）时不注册：这些端点不是站点主干，
		// 缺了不该让整个路由树 panic。
		return
	}
	app := moderationHandler

	group := r.Group("moderation")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", middleware.AdminMiddleware)

	authGroup.GET("me", app.MySummary)
	authGroup.POST("me/appeals", app.SubmitAppeal)

	adminGroup.GET("appeals", app.ListAppeals)
	adminGroup.POST("appeals/:id/handle", app.HandleAppeal)
}
