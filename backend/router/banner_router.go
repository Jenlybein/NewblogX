package router

import (
	"myblogx/api"
	"myblogx/api/banner_api"
	mw "myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

func BannerRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	Group := r.Group("banners")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	app := appContainer.BannerApi

	Group.GET("", mw.BindQuery[banner_api.BannerListRequest], app.BannerListView)
	adminGroup.POST("", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[banner_api.BannerCreateRequest], app.BannerCreateView)
	adminGroup.PUT(":id", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindUri[models.IDRequest], mw.BindJson[banner_api.BannerCreateRequest], app.BannerUpdateView)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[models.IDListRequest], app.BannerRemoveView)
}
