package middleware

import (
	"myblogx/common/res"
	"myblogx/utils/io_util"

	"github.com/gin-gonic/gin"
)

type CaptchaMiddlewareRequest struct {
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
}

func CaptchaMiddleware(c *gin.Context) {
	runtimeFromContext(c).CaptchaMiddleware(c)
}

func (h Runtime) CaptchaMiddleware(c *gin.Context) {
	if h.RuntimeSite == nil || !h.RuntimeSite.GetRuntimeLogin().Captcha {
		return
	}

	var cr CaptchaMiddlewareRequest
	if err := io_util.ShouldBindJSONWithRecover(c, &cr); err != nil {
		if h.Logger != nil {
			h.Logger.Errorf("图形验证失败：请求体绑定失败：%v", err)
		}
		res.FailWithMsg("图形验证失败：请求体绑定失败", c)
		c.Abort()
		return
	}

	if h.ImageCaptchaStore == nil || !h.ImageCaptchaStore.Verify(cr.CaptchaID, cr.CaptchaCode, true) {
		res.FailWithMsg("图形验证码错误", c)
		c.Abort()
		return
	}
}
