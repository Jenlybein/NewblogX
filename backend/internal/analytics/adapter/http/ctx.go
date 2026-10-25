package http

import (
	"context"

	"github.com/gin-gonic/gin"
)

// reqCtx 取请求上下文。
//
// gin.CreateTestContext 造出来的 Context 没有 Request，
// 直接用 c.Request.Context() 会在单元测试里空指针。
func reqCtx(c *gin.Context) context.Context {
	if c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
