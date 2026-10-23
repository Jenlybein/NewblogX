package auth_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/enum"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
)

func (h AuthApi) RefreshTokenView(c *gin.Context) {
	deps := user_service.NewDepsWithRedis(h.App.JWT, h.App.System.Env, h.App.DB, h.App.Logger, h.App.Redis)
	// 获取旧的刷新令牌
	refreshToken := user_service.GetRefreshTokenByRequest(c.Request)

	// 用旧的刷新令牌换取新的AccessToken和新的刷新令牌
	accessToken, newRefreshToken, _, _, err := user_service.RefreshTokens(deps, refreshToken, buildSessionMeta(c))

	if err != nil {
		middleware.EmitLoginEventFromGin(c, "token_refresh", enum.LoginType(0), false, "", 0, err.Error(), nil)
		user_service.ClearRefreshTokenCookie(c.Writer, deps)
		res.FailWithMsg(err.Error(), c)
		return
	}
	user_service.SetRefreshTokenCookie(c.Writer, newRefreshToken, deps)
	middleware.EmitLoginEventFromGin(c, "token_refresh", enum.LoginType(0), true, "", 0, "", nil)

	res.OkWithData(accessToken, c)
}
