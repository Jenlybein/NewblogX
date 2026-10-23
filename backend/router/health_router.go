package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthRouter 注册仅用于进程存活探测的轻量接口。
// 该接口不依赖数据库、缓存、搜索等外部服务。
func HealthRouter(r gin.IRoutes) {
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
