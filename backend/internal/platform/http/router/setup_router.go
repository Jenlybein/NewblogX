package router

import (
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// SetupRouter 注册首次初始化路由。
//
// 这两个接口**故意不带鉴权中间件**：它们存在的意义就是"还没有任何账号
// 可以登录"。安全边界完全靠用例层的闸门（库里有没有管理员），
// 一旦初始化完成就永久返回 setup.already_done。
//
// 也因此不能把它们挂在 users 组下：那个组里的中间件是给"已登录用户"用的，
// 语义上不属于这条路。
func SetupRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if userHandler == nil {
		panic("router: 用户处理器未注入（应在 app.WireUser 中调用 SetUserHandler）")
	}
	_ = runtimeMw // 保持与其它 Router 函数一致的签名，本路由不需要任何中间件

	group := r.Group("setup")
	group.GET("status", userHandler.SetupStatus)
	group.POST("admin", userHandler.SetupAdmin)
}
