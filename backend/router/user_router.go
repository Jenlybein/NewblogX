package router

import (
	"myblogx/api"
	authhttp "myblogx/internal/auth/adapter/http"
	userhttp "myblogx/internal/user/adapter/http"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// authHandler 是认证模块的 HTTP 处理器，由 app.WireUser 在启动时注入。
//
// 与 article 同一套路：路由文件不再自己 new 处理器，装配职责集中在 app 层。
var (
	authHandler *authhttp.Handler
	userHandler *userhttp.Handler
)

// SetAuthHandler 注入认证处理器。
func SetAuthHandler(h *authhttp.Handler) { authHandler = h }

// SetUserHandler 注入用户模块处理器。
func SetUserHandler(h *userhttp.Handler) { userHandler = h }

// UserRouter 注册用户与认证路由。
//
// 认证与用户资料/管理端都已迁到 internal/*；登录日志暂留旧 api（归 S4.9 analytics）。
func UserRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	if authHandler == nil {
		panic("router: 认证处理器未注入（应在 app.WireUser 中调用 SetAuthHandler）")
	}
	if userHandler == nil {
		panic("router: 用户处理器未注入（应在 app.WireUser 中调用 SetUserHandler）")
	}

	Group := r.Group("users")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	auth := authHandler
	Group.POST("email/verify", runtimeMw.CaptchaMiddleware, auth.SendEmail)
	Group.POST("email/login", runtimeMw.EmailVerifyMiddleware, auth.EmailLogin)
	Group.POST("email/register", runtimeMw.EmailVerifyMiddleware, auth.RegisterEmail)
	Group.POST("qq", auth.QQLogin)
	Group.POST("login", runtimeMw.CaptchaMiddleware, auth.PwdLogin)
	Group.POST("refresh", auth.Refresh)
	Group.PUT("password/recovery/email", runtimeMw.EmailVerifyMiddleware, auth.ResetPassword)
	authGroup.PUT("password/renewal/email", auth.UpdatePassword)
	authGroup.POST("logout", auth.Logout)
	authGroup.POST("logout/all", auth.LogoutAll)
	authGroup.PUT("email/bind", runtimeMw.EmailVerifyMiddleware, auth.BindEmail)
	authGroup.GET("sessions", auth.Sessions)
	authGroup.DELETE("sessions/:id", auth.DeleteSession)

	profile := userHandler
	authGroup.GET("detail", profile.Detail)
	Group.GET("base", runtimeMw.OptionalAuthMiddleware, profile.BaseInfo)
	authGroup.PUT("info", profile.UpdateInfo)
	adminGroup.PUT("admin/info", mw.CaptureLog(mw.ReqBody), profile.AdminUpdate)

	log := appContainer.UserApi.LogApi
	adminGroup.GET("login/log", log.UserLoginLogList)

	adminGroup.POST("admin", profile.AdminCreate)
	adminGroup.GET("admin/list", profile.AdminList)
}
