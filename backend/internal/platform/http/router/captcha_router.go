package router

import (
	"myblogx/internal/platform/captcha"

	"github.com/gin-gonic/gin"
)

// captchaHandler 由 app.WireCaptcha 在启动时注入。
var captchaHandler *captcha.Handler

// SetCaptchaHandler 注入验证码处理器。
func SetCaptchaHandler(h *captcha.Handler) { captchaHandler = h }

// CaptchaRouter 注册验证码路由。
//
// 这是最后一个进 api.Api 容器的处理器；容器随之删除，路由文件也不再接收
// api.Api 参数 —— 与 article / user 等模块的注入式写法统一。
func CaptchaRouter(r *gin.RouterGroup) {
	if captchaHandler == nil {
		panic("router: 验证码处理器未注入（应在 app.WireCaptcha 中调用 SetCaptchaHandler）")
	}
	r.GET("/imagecaptcha", captchaHandler.Image)
}
