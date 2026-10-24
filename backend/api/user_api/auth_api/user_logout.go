package auth_api

import (
	"myblogx/common/res"
	"myblogx/internal/platform/redis/redis_jwt"
	"myblogx/middleware"
	"myblogx/models/enum"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// UserLogoutView 退出当前设备登录。
// @Summary 退出登录
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /users/logout [post]
func (h AuthApi) UserLogoutView(c *gin.Context) {
	claims := jwts.MustGetClaimsByGin(c)
	deps := user_service.NewDepsWithRedis(h.App.JWT, h.App.System.Env, h.App.DB, h.App.Logger, h.App.Redis)
	if err := user_service.RevokeSessionByID(deps, claims.UserID, claims.SessionID); err != nil {
		middleware.EmitLoginEventFromGin(c, "logout", enum.LoginType(0), false, claims.Username, claims.UserID, err.Error(), nil)
		res.FailWithError(err, c)
		return
	}

	token := jwts.GetTokenByGin(c)
	if token != "" {
		redis_jwt.SetTokenBlack(deps.Redis, deps.JWT, token, redis_jwt.UserBlackType)
	}
	user_service.ClearRefreshTokenCookie(c.Writer, deps)
	middleware.EmitLoginEventFromGin(c, "logout", enum.LoginType(0), true, claims.Username, claims.UserID, "", nil)

	res.OkWithMsg("退出登录成功", c)
}

// UserLogoutAllView 退出全部设备登录。
// @Summary 退出全部设备
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /users/logout/all [post]
func (h AuthApi) UserLogoutAllView(c *gin.Context) {
	claims := jwts.MustGetClaimsByGin(c)
	deps := user_service.NewDepsWithRedis(h.App.JWT, h.App.System.Env, h.App.DB, h.App.Logger, h.App.Redis)
	if err := user_service.RevokeAllUserSessions(deps, claims.UserID); err != nil {
		middleware.EmitLoginEventFromGin(c, "logout_all", enum.LoginType(0), false, claims.Username, claims.UserID, err.Error(), nil)
		res.FailWithError(err, c)
		return
	}

	token := jwts.GetTokenByGin(c)
	if token != "" {
		redis_jwt.SetTokenBlack(deps.Redis, deps.JWT, token, redis_jwt.UserBlackType)
	}
	user_service.ClearRefreshTokenCookie(c.Writer, deps)
	middleware.EmitLoginEventFromGin(c, "logout_all", enum.LoginType(0), true, claims.Username, claims.UserID, "", nil)

	res.OkWithMsg("已退出全部设备", c)
}
