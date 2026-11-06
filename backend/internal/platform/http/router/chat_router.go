package router

import (
	chathttp "myblogx/internal/chat/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// ChatRouter 注册私信路由。
//
// 处理器由合成根显式传入（#280 清掉这一处包级可变状态）。
// 说明：本仓库其它模块仍用 xxxHandler + SetXxxHandler 的包级注入，属于存量；
// 本次只清私信这一处，不宣称全仓已无包级可变状态。
func ChatRouter(r *gin.RouterGroup, runtimeMw mw.Runtime, handler *chathttp.Handler) {
	if handler == nil {
		panic("router: 私信处理器未注入（应由 internal/chat.New 装配后经 router.Handlers 传入）")
	}
	app := handler

	group := r.Group("chat")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	authGroup.GET("sessions", app.Sessions)
	authGroup.DELETE("sessions", app.DeleteSessions)
	authGroup.GET("messages", app.Messages)
	// 发送的唯一通道：HTTP 可靠落库（幂等）。WS 只做通知，不再接受上行消息。
	authGroup.POST("messages", app.Send)
	authGroup.POST("read", app.Read)
	authGroup.DELETE("messages", app.DeleteMessages)
	authGroup.GET("ws-ticket", app.WsTicket)
	group.GET("ws", app.Ws)
}
