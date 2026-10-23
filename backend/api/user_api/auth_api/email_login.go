package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
)

// EmailLoginView 邮箱验证码登录。
// 这里依赖 EmailVerifyMiddleware 先完成验证码校验，并把邮箱写入上下文。
func (h AuthApi) EmailLoginView(c *gin.Context) {
	app := h.App
	if app.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	if !app.RuntimeSite.GetRuntimeLogin().EmailLogin {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.EmailLoginType, false, "", 0, "站点未启用邮箱登录", nil)
		res.FailWithMsg("站点未启用邮箱登录功能", c)
		return
	}

	email := c.GetString("email")
	if email == "" {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.EmailLoginType, false, "", 0, "邮箱验证失败：邮箱不存在", nil)
		res.FailWithMsg("邮箱验证失败：邮箱不存在", c)
		return
	}

	var user models.UserModel
	if err := app.DB.Take(&user, "email = ?", email).Error; err != nil {
		// 这里不区分“邮箱不存在”和其他细节，避免把账号状态暴露给外部调用方。
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.EmailLoginType, false, email, 0, "邮箱登录失败", map[string]any{
			"username": email,
		})
		res.FailWithMsg("邮箱登录失败", c)
		return
	}
	if !user.CanLogin() {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.EmailLoginType, false, user.Username, user.ID, user.Status.String(), map[string]any{
			"username": user.Username,
		})
		res.FailWithMsg(user.Status.String(), c)
		return
	}

	deps := user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis)
	accessToken, refreshToken, _, err := user_service.CreateLoginTokens(deps, &user, buildSessionMeta(c))
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.EmailLoginType, false, user.Username, user.ID, "邮箱登录失败", map[string]any{
			"username": user.Username,
		})
		res.FailWithMsg("邮箱登录失败", c)
		return
	}

	user_service.SetRefreshTokenCookie(c.Writer, refreshToken, deps)
	middleware.EmitLoginEventFromGin(c, "login_success", enum.EmailLoginType, true, user.Username, user.ID, "", map[string]any{
		"username": user.Username,
	})

	res.OkWithData(accessToken, c)
}
