package router

import (
	"myblogx/api"
	"myblogx/api/image_api"
	"myblogx/common"
	mw "myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

func ImageRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	Group := r.Group("images")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	app := appContainer.ImageApi

	authGroup.POST("upload-tasks", mw.BindJson[image_api.CreateImageUploadTaskRequest], app.CreateUploadTaskView)
	authGroup.POST("upload-tasks/complete", mw.BindJson[image_api.CompleteImageUploadTaskRequest], app.CompleteUploadTaskView)
	authGroup.GET("upload-tasks/:id", mw.BindUri[models.IDRequest], app.UploadTaskStatusView)
	Group.POST("qiniu/callback", app.QiniuCallbackView)
	Group.POST("qiniu/audit/callback", app.QiniuAuditCallbackView)

	adminGroup.GET("", mw.BindQuery[common.PageInfo], app.ImageListView)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[models.IDListRequest], app.ImageRemoveView)
}
