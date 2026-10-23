package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_user"
	"myblogx/service/user_service"
	"myblogx/utils/pwd"
	"strings"

	"github.com/gin-gonic/gin"
)

type PwdLoginRequest struct {
	Username string `json:"username" binding:"required"` // 用户名或邮箱
	Password string `json:"password" binding:"required"`
}

func (h AuthApi) PwdLoginView(c *gin.Context) {
	app := h.App
	runtimeSite := app.RuntimeSite
	if runtimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	loginConf := runtimeSite.GetRuntimeLogin()
	if !loginConf.UsernamePwdLogin {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.PasswordLoginType, false, "", 0, "站点未启用密码登录", nil)
		res.FailWithMsg("站点未启用密码登录功能", c)
		return
	}

	cr := middleware.GetBindJson[PwdLoginRequest](c)
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)

	// 登录失败次数限制
	meta := buildSessionMeta(c)
	account := strings.TrimSpace(cr.Username)
	if !redis_user.CheckLoginAllowed(redisDeps, account, meta.IP) {
		middleware.EmitLoginEventFromGin(c, "login_risk_control", enum.PasswordLoginType, false, account, 0, user_service.ErrLoginTooFrequent.Error(), map[string]any{
			"username": account,
		})
		res.FailWithMsg(user_service.ErrLoginTooFrequent.Error(), c)
		return
	}

	var user models.UserModel
	if err := app.DB.Take(
		&user,
		"(username = ? OR email = ?) and (password <> '')",
		account, account,
	).Error; err != nil {
		// Redis 记录登录失败
		redis_user.RecordLoginFailure(redisDeps, loginConf, account, meta.IP)
		// 日志记录登录失败
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.PasswordLoginType, false, account, 0, "账号或密码错误", map[string]any{
			"username": account,
		})
		res.FailWithMsg("账号或密码错误", c)
		return
	}

	// 校验密码
	if !pwd.CompareHashAndPassword(user.Password, cr.Password) {
		redis_user.RecordLoginFailure(redisDeps, loginConf, account, meta.IP)
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.PasswordLoginType, false, account, ctype.ID(user.ID), "账号或密码错误", map[string]any{
			"username": account,
		})
		res.FailWithMsg("账号或密码错误", c)
		return
	}

	// 账号被禁用
	if !user.CanLogin() {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.PasswordLoginType, false, user.Username, user.ID, user.Status.String(), map[string]any{
			"username": user.Username,
		})
		res.FailWithMsg(user.Status.String(), c)
		return
	}

	// Redis 记录登录成功
	redis_user.ResetLoginFailures(redisDeps, account, meta.IP)

	// 登录成功后创建服务端会话，再签发短期访问令牌。
	deps := user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis)
	token, refreshToken, _, err := user_service.CreateLoginTokens(deps, &user, meta)
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.PasswordLoginType, false, user.Username, user.ID, err.Error(), map[string]any{
			"username": user.Username,
		})
		res.FailWithError(err, c)
		return
	}

	// 设置刷新令牌
	user_service.SetRefreshTokenCookie(c.Writer, refreshToken, deps)

	// 记录登录日志
	middleware.EmitLoginEventFromGin(c, "login_success", enum.PasswordLoginType, true, user.Username, user.ID, "", map[string]any{
		"username": user.Username,
	})

	res.OkWithData(token, c)
}
