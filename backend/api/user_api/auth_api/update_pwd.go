package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"myblogx/utils/pwd"

	"github.com/gin-gonic/gin"
)

type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h AuthApi) UpdatePwdByEmailView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[UpdatePasswordRequest](c)

	claims := jwts.MustGetClaimsByGin(c)

	var user models.UserModel
	if err := app.DB.Take(&user, claims.UserID).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}

	// 邮箱注册 or 已绑定邮箱的用户
	if user.Email == nil || *user.Email == "" {
		res.FailWithMsg("用户未绑定邮箱", c)
		return
	}

	// 校验旧密码
	if !pwd.CompareHashAndPassword(user.Password, cr.OldPassword) {
		res.FailWithMsg("旧密码错误", c)
		return
	}

	// 校验新密码
	if cr.NewPassword == cr.OldPassword {
		res.FailWithMsg("新密码不能与旧密码相同", c)
		return
	}

	// 设置新密码
	hashPwd, err := pwd.GenerateFromPassword(cr.NewPassword)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	deps := user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis)
	if err := user_service.UpdatePasswordAndRevokeSessions(deps, &user, hashPwd); err != nil {
		res.FailWithError(err, c)
		return
	}
	user_service.ClearRefreshTokenCookie(c.Writer, deps)

	res.OkWithMsg("密码更新成功", c)
}
