package router

import (
	"myblogx/api"
	"myblogx/api/follow_api"
	mw "myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

func FollowRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	app := appContainer.FollowApi

	// 关注
	followGroup := r.Group("follow")
	followAuthGroup := followGroup.Group("", runtimeMw.AuthMiddleware)
	// followAdminGroup := followAuthGroup.Group("", mw.AdminMiddleware)

	followAuthGroup.POST(":id", mw.BindUri[models.IDRequest], app.FollowUserView)
	followAuthGroup.DELETE(":id", mw.BindUri[models.IDRequest], app.UnfollowUserView)
	followAuthGroup.GET("", mw.BindQuery[follow_api.FollowListRequest], app.FollowListView)

	// 粉丝
	fansGroup := r.Group("fans")
	FansAuthGroup := fansGroup.Group("", runtimeMw.AuthMiddleware)

	FansAuthGroup.GET("", mw.BindQuery[follow_api.FansListRequest], app.FansListView)
}
