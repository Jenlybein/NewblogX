package middleware

import (
	"myblogx/common/res"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(c *gin.Context) {
	runtimeFromContext(c).AuthMiddleware(c)
}

func OptionalAuthMiddleware(c *gin.Context) {
	runtimeFromContext(c).OptionalAuthMiddleware(c)
}

func AdminMiddleware(c *gin.Context) {
	claims := jwts.MustGetClaimsByGin(c)

	if !claims.IsAdmin() {
		res.FailWithMsg("权限错误", c)
		c.Abort()
		return
	}
	c.Next()
}

func (h Runtime) AuthMiddleware(c *gin.Context) {
	if h.Authenticator == nil {
		res.FailWithMsg("鉴权依赖未初始化", c)
		c.Abort()
		return
	}

	authResult, err := h.Authenticator.AuthenticateAccessToken(jwts.GetTokenByGin(c))
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		c.Abort()
		return
	}

	c.Set("claims", authResult.Claims)
	c.Set("auth_user", authResult.User)
	c.Set("auth_session", authResult.Session)
	c.Set("access_token", authResult.Token)
	c.Next()
}

func (h Runtime) OptionalAuthMiddleware(c *gin.Context) {
	if h.Authenticator == nil {
		c.Next()
		return
	}

	token := jwts.GetTokenByGin(c)
	if token == "" {
		c.Next()
		return
	}

	authResult, err := h.Authenticator.AuthenticateAccessToken(token)
	if err != nil {
		c.Next()
		return
	}

	c.Set("claims", authResult.Claims)
	c.Set("auth_user", authResult.User)
	c.Set("auth_session", authResult.Session)
	c.Set("access_token", authResult.Token)
	c.Next()
}
