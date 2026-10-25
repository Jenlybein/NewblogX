package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CorsMiddleware 统一处理浏览器跨域请求。
// HTTP CORS 和 WebSocket 共用 Origin 白名单，避免前后策略漂移。
func CorsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowFiles:      false,
		AllowOriginFunc: IsAllowedOrigin,
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"HEAD",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Content-Length",
			"Accept",
			"Authorization",
			"token",
			"X-Requested-With",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
		},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	})
}
