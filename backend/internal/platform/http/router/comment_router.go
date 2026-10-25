package router

import (
	commenthttp "myblogx/internal/comment/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// commentHandler 由 app.WireComment 在启动时注入。
var commentHandler *commenthttp.Handler

// SetCommentHandler 注入评论处理器。
func SetCommentHandler(h *commenthttp.Handler) { commentHandler = h }

// CommentRouter 注册评论路由。
func CommentRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if commentHandler == nil {
		panic("router: 评论处理器未注入（应在 app.WireComment 中调用 SetCommentHandler）")
	}
	app := commentHandler

	Group := r.Group("comments")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)

	Group.GET("", app.ListRoot)
	Group.GET("replies", app.ListReplies)
	authGroup.GET("man", app.ListManaged)
	authGroup.POST("", app.Create)
	authGroup.POST(":id/digg", app.Digg)
	authGroup.DELETE(":id", app.Remove)
}
