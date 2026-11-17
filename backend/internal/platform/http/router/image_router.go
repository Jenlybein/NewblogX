package router

import (
	mediahttp "myblogx/internal/media/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// imageHandler 由 app.WireMedia 在启动时注入。
var imageHandler *mediahttp.Handler

// SetImageHandler 注入图片处理器。
func SetImageHandler(h *mediahttp.Handler) { imageHandler = h }

// ImageRouter 注册图片路由。
func ImageRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if imageHandler == nil {
		panic("router: 图片处理器未注入（应在 app.WireMedia 中调用 SetImageHandler）")
	}
	app := imageHandler

	Group := r.Group("images")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	authGroup.POST("upload-tasks", app.CreateUploadTask)
	authGroup.POST("upload-tasks/complete", app.CompleteUploadTask)
	authGroup.GET("upload-tasks/:id", app.UploadTaskStatus)
	Group.POST("qiniu/callback", app.QiniuCallback)
	Group.POST("qiniu/audit/callback", app.QiniuAuditCallback)

	adminGroup.GET("", app.List)
	// 样式探活（ADR-0034）：保存样式名后点一下就知道填没填对。
	adminGroup.POST("style-probe", app.ProbeStyles)
	// 人工审核（ADR-0034）：待审核/疑似的图由管理员放行或驳回。
	adminGroup.POST(":id/audit", app.Audit)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody), app.Remove)
}
