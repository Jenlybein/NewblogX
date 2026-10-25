package middleware

import (
	"context"

	"myblogx/internal/platform/http/res"
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
	if h.Verifier == nil {
		res.FailWithMsg("鉴权依赖未初始化", c)
		c.Abort()
		return
	}

	token := jwts.GetTokenByGin(c)
	claims, err := h.Verifier.VerifyAccessToken(requestContext(c), token)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		c.Abort()
		return
	}

	// 只放 claims：它是下游唯一被读的东西（GetClaimsByGin / MustGetClaimsByGin）。
	// 原先还写了 auth_user / auth_session（完整用户行与会话行），全仓无人读取，
	// 属于把整行数据塞进请求上下文却没人用的死代码，一并去掉。
	c.Set("claims", claims)
	c.Set("access_token", token)
	c.Next()
}

func (h Runtime) OptionalAuthMiddleware(c *gin.Context) {
	if h.Verifier == nil {
		c.Next()
		return
	}

	token := jwts.GetTokenByGin(c)
	if token == "" {
		c.Next()
		return
	}

	claims, err := h.Verifier.VerifyAccessToken(requestContext(c), token)
	if err != nil {
		c.Next()
		return
	}

	c.Set("claims", claims)
	c.Set("access_token", token)
	c.Next()
}

// requestContext 取请求上下文。
//
// gin.CreateTestContext 造出来的 Context 没有 Request，直接用 c.Request.Context()
// 会在单元测试里空指针（analytics 那边踩过同样的坑）。
func requestContext(c *gin.Context) context.Context {
	if c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
