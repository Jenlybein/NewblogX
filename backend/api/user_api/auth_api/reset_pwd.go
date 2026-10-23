package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/user_service"
	"myblogx/utils/pwd"

	"github.com/gin-gonic/gin"
)

type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

func (h AuthApi) ResetPwdByEmailView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[ResetPasswordRequest](c)

	email := c.GetString("email")

	var user models.UserModel
	var err error
	if err = app.DB.Take(&user, "email = ?", email).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}

	// 校验旧密码
	if pwd.CompareHashAndPassword(user.Password, cr.NewPassword) {
		res.FailWithMsg("新密码不能与旧密码相同", c)
		return
	}

	// 设置新密码
	hashPwd, err := pwd.GenerateFromPassword(cr.NewPassword)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	if err := user_service.UpdatePasswordAndRevokeSessions(user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis), &user, hashPwd); err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithMsg("密码重置成功", c)
}
