package router

import (
	notificationhttp "myblogx/internal/notification/adapter/http"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// notificationHandler 由 app.WireNotification 在启动时注入。
//
// 站内信与全局公告共用一个处理器，所以变量放在这里而不是各自的文件里。
var notificationHandler *notificationhttp.Handler

// SetNotificationHandler 注入通知处理器。
func SetNotificationHandler(h *notificationhttp.Handler) { notificationHandler = h }

// mustNotificationHandler 取处理器，未注入直接 panic（与 article 一致）。
func mustNotificationHandler() *notificationhttp.Handler {
	if notificationHandler == nil {
		panic("router: 通知处理器未注入（应在 app.WireNotification 中调用 SetNotificationHandler）")
	}
	return notificationHandler
}

// GlobalNotifRouter 注册全局公告路由。
func GlobalNotifRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustNotificationHandler()

	group := r.Group("global_notif")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	authGroup.GET("", mw.BindQuery[notificationhttp.GlobalListRequest], app.GlobalList)
	authGroup.POST("read", mw.BindJson[notificationhttp.IDListRequest], app.GlobalRead)
	authGroup.DELETE("user", mw.BindJson[notificationhttp.IDListRequest], app.GlobalUserRemove)

	adminGroup.POST("", mw.CaptureLog(mw.ReqBody), mw.BindJson[notificationhttp.GlobalCreateRequest], app.GlobalCreate)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody), mw.BindJson[notificationhttp.IDListRequest], app.GlobalAdminRemove)
}

// SitemsgRouter 注册站内信路由。
func SitemsgRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustNotificationHandler()

	group := r.Group("sitemsg")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)

	authGroup.GET("", mw.BindQuery[notificationhttp.MessageListRequest], app.Messages)
	authGroup.POST("", mw.BindJson[notificationhttp.MessageSelectRequest], app.MessageRead)
	authGroup.DELETE("", mw.BindJson[notificationhttp.MessageSelectRequest], app.MessageRemove)

	authGroup.GET("conf", app.Conf)
	authGroup.PUT("conf", mw.BindJson[notificationhttp.ConfRequest], app.ConfUpdate)

	authGroup.GET("user", app.Unread)
}
