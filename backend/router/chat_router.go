package router

import (
	"myblogx/api"
	"myblogx/api/chat_api"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func ChatRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	group := r.Group("chat")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)

	app := appContainer.ChatApi
	authGroup.GET("sessions", mw.BindQuery[chat_api.ChatSessionListRequest], app.ChatSessionListView)
	authGroup.DELETE("sessions", mw.BindJson[chat_api.ChatSessionDeleteUserRequest], app.ChatSessionDeleteUserView)
	authGroup.GET("messages", mw.BindQuery[chat_api.ChatMsgListRequest], app.ChatMsgListView)
	authGroup.POST("read", mw.BindJson[chat_api.ChatMsgReadUserRequest], app.ChatMsgReadUserView)
	authGroup.DELETE("messages", mw.BindJson[chat_api.ChatMsgDeleteUserRequest], app.ChatMsgDeleteUserView)
	authGroup.GET("ws-ticket", app.ChatWsTicketView)
	group.GET("ws", app.ChatWsView)
}
