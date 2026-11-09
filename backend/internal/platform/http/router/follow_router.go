package router

import (
	mw "myblogx/internal/platform/http/middleware"
	socialhttp "myblogx/internal/social/adapter/http"

	"github.com/gin-gonic/gin"
)

// followHandler 由 app.WireSocial 在启动时注入。
var followHandler *socialhttp.Handler

// SetFollowHandler 注入社交处理器。
func SetFollowHandler(h *socialhttp.Handler) { followHandler = h }

// FollowRouter 注册关系读写路由。
//
// 路径按"写 / 读"分开（票 327）：
//
//	/follow/{id}           POST/DELETE  我对某人的动作（关注 / 取关）
//	/relations             GET          关系视图（关注 / 粉丝 / 互关，同一个形状）
//	/relations/counts      GET          三个视图的计数（顶栏徽标）
//	/relations/fans/{id}   DELETE       单方面移除一个粉丝
//
// 读为什么不再分两个路径：三个视图是同一个查询的三种参数，分成两个端点后
// 前端不得不写两套归一、并且要自己在客户端筛互关——那正是票 327 要消掉的东西。
func FollowRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if followHandler == nil {
		panic("router: 社交处理器未注入（应在 app.WireSocial 中调用 SetFollowHandler）")
	}
	app := followHandler

	followAuth := r.Group("follow").Group("", runtimeMw.AuthMiddleware)
	followAuth.POST(":id", app.Follow)
	followAuth.DELETE(":id", app.Unfollow)

	relationAuth := r.Group("relations").Group("", runtimeMw.AuthMiddleware)
	relationAuth.GET("", app.ListRelations)
	relationAuth.GET("counts", app.RelationCounts)
	relationAuth.DELETE("fans/:id", app.RemoveFan)
}
