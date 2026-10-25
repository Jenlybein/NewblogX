package router

import (
	chathttp "myblogx/internal/chat/adapter/http"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// chatHandler 由 app.WireChat 在启动时注入。
var chatHandler *chathttp.Handler

// SetChatHandler 注入私信处理器。
func SetChatHandler(h *chathttp.Handler) { chatHandler = h }

// ChatRouter 注册私信路由。
func ChatRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if chatHandler == nil {
		panic("router: 私信处理器未注入（应在 app.WireChat 中调用 SetChatHandler）")
	}
	app := chatHandler

	group := r.Group("chat")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	authGroup.GET("sessions", app.Sessions)
	authGroup.DELETE("sessions", app.DeleteSessions)
	authGroup.GET("messages", app.Messages)
	authGroup.POST("read", app.Read)
	authGroup.DELETE("messages", app.DeleteMessages)
	authGroup.GET("ws-ticket", app.WsTicket)
	group.GET("ws", app.Ws)
}
