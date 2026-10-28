package router

import (
	"net/http"

	"myblogx/internal/platform/health"

	"github.com/gin-gonic/gin"
)

// HealthRouter 注册健康检查接口。分两个，语义严格区分：
//
//	/health        存活（liveness）：进程还能处理请求就是 200，不看任何组件。
//	/health/ready  就绪（readiness）：后台组件里有 failed 就返回 503。
//
// # 为什么要把它们分开
//
// 合并成一个的话只有两种选择，两种都错：
//   - 都看组件状态 → CDC 挂掉让容器判定不健康，编排层可能重启进程，
//     但重启解决不了"复制账号密码错"这类问题，只会放大故障。
//   - 都不看组件状态 → 就是事故当时的样子：river 死了，探针全绿。
//
// registry 允许为 nil（只跑 HTTP 的角色没有后台组件），此时就绪检查恒为健康。
func HealthRouter(r gin.IRoutes, registry *health.Registry) {
	// HealthHandler 进程存活探测。
	// @Summary 进程存活探测
	// @Tags system
	// @Success 200 "服务存活"
	// @Router /health [get]
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	r.GET("/health/ready", func(c *gin.Context) {
		components := registry.Snapshot()
		healthy := registry.Healthy()

		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}

		// components 为 nil 时编码成 null，前端/探针要能处理这个形状，
		// 所以显式换成空切片，保证它永远是数组。
		if components == nil {
			components = []health.Component{}
		}

		c.JSON(status, gin.H{
			"healthy":    healthy,
			"components": components,
		})
	})
}
