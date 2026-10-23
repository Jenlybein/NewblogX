package middleware

import (
	"myblogx/common/res"
	redis_email "myblogx/service/redis_service/redis_email"
	"myblogx/utils/io_util"

	"github.com/gin-gonic/gin"
)

type EmailVerifyMiddlewareRequest struct {
	EmailID   string `json:"email_id" binding:"required"`
	EmailCode string `json:"email_code" binding:"required"`
}

func EmailVerifyMiddleware(c *gin.Context) {
	runtimeFromContext(c).EmailVerifyMiddleware(c)
}

func (h Runtime) EmailVerifyMiddleware(c *gin.Context) {
	// 读取并恢复请求体
	var cr EmailVerifyMiddlewareRequest
	if err := io_util.ShouldBindJSONWithRecover(c, &cr); err != nil {
		if h.Logger != nil {
			h.Logger.Errorf("邮箱验证失败：请求体绑定失败：%v", err)
		}
		res.FailWithMsg("邮箱验证失败：请求体读取失败", c)
		c.Abort()
		return
	}

	email, ok, err := redis_email.Verify(h.Redis, cr.EmailID, cr.EmailCode)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Errorf("邮箱验证失败：校验异常：%v", err)
		}
		res.FailWithMsg("邮箱验证失败", c)
		c.Abort()
		return
	}
	if !ok {
		res.FailWithMsg("邮箱验证失败：验证码不存在或错误", c)
		c.Abort()
		return
	}

	c.Set("email", email)
}
