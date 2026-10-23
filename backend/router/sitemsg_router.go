package router

import (
	"myblogx/api"
	"myblogx/api/sitemsg_api"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func SitemsgRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	Group := r.Group("sitemsg")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	// adminGroup := authGroup.Group("", mw.AdminMiddleware)

	app := appContainer.SitemsgApi
	authGroup.GET("", mw.BindQuery[sitemsg_api.SitemsgListRequest], app.SitemsgListView)
	authGroup.POST("", mw.BindJson[sitemsg_api.SitemsgReadRequest], app.SitemsgReadView)
	authGroup.DELETE("", mw.BindJson[sitemsg_api.SitemsgRemoveRequest], app.SitemsgRemoveView)

	authGroup.GET("conf", app.UserMsgConfView)
	authGroup.PUT("conf", mw.BindJson[sitemsg_api.UserMsgConfResponseAndRequest], app.UserMsgConfUpdateView)

	authGroup.GET("user", app.SitemsgUserView)
}
