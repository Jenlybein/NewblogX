// 路由模块入口

package router

import (
	"myblogx/api"
	"myblogx/apideps"
	"myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func Run(deps apideps.Deps, app api.Api) {
	gin.SetMode(deps.System.GinMode)
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	r.Use(gin.Recovery())
	r.Use(middleware.CorsMiddleware())
	mwRuntime := middleware.NewRuntime(deps)

	r.Static("/uploads", "./uploads")
	HealthRouter(r)

	nr := r.Group("/api")
	nr.Use(mwRuntime.LogMiddleware)

	SiteRouter(nr, app, mwRuntime)
	LogRouter(nr, app, mwRuntime)
	ImageRouter(nr, app, mwRuntime)
	BannerRouter(nr, app, mwRuntime)
	CaptchaRouter(nr, app, mwRuntime)
	UserRouter(nr, app, mwRuntime)
	ArticleRouter(nr, app, mwRuntime)
	CommentRouter(nr, app, mwRuntime)
	ChatRouter(nr, app, mwRuntime)
	SitemsgRouter(nr, app, mwRuntime)
	GlobalNotifRouter(nr, app, mwRuntime)
	FollowRouter(nr, app, mwRuntime)
	SearchRouter(nr, app, mwRuntime)
	AIRouter(nr, app, mwRuntime)
	DataRouter(nr, app, mwRuntime)

	addr := deps.System.Addr()
	r.Run(addr)
}
