package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthRouter 注册仅用于进程存活探测的轻量接口。
// 该接口不依赖数据库、缓存、搜索等外部服务。
func HealthRouter(r gin.IRoutes) {
	// HealthHandler 进程存活探测。
	// @Summary 进程存活探测
	// @Tags system
	// @Success 200 "服务存活"
	// @Router /health [get]
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
