package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/enum"
	"myblogx/service/redis_service/redis_jwt"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

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
