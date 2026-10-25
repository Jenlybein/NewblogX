package router

import (
	socialhttp "myblogx/internal/social/adapter/http"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// followHandler 由 app.WireSocial 在启动时注入。
var followHandler *socialhttp.Handler

// SetFollowHandler 注入社交处理器。
func SetFollowHandler(h *socialhttp.Handler) { followHandler = h }

// FollowRouter 注册关注与粉丝路由。
func FollowRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if followHandler == nil {
		panic("router: 社交处理器未注入（应在 app.WireSocial 中调用 SetFollowHandler）")
	}
	app := followHandler

	followAuth := r.Group("follow").Group("", runtimeMw.AuthMiddleware)
	followAuth.POST(":id", app.Follow)
	followAuth.DELETE(":id", app.Unfollow)
	followAuth.GET("", app.ListFollowing)

	fansAuth := r.Group("fans").Group("", runtimeMw.AuthMiddleware)
	fansAuth.GET("", app.ListFans)
}
