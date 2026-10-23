package router

import (
	"myblogx/api"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func CaptchaRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	api := appContainer.ImageCaptchaApi
	r.GET("/imagecaptcha", api.CaptchaView)
}
