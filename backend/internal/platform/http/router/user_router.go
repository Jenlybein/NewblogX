package router

import (
	authhttp "myblogx/internal/auth/adapter/http"
	mw "myblogx/internal/platform/http/middleware"
	userhttp "myblogx/internal/user/adapter/http"

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
// 认证、用户资料、管理端、登录日志分别来自 internal/auth、internal/user、internal/analytics。
func UserRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
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
	// 图形验证码按**入口**单独挂闸门（第 12 段 / ADR-0033）：发码卡成本、登录卡爆破，
	// 两者被滥用的方式不同，所以各自有一个开关，而不是共用一个"全部校验"。
	Group.POST("email/verify", mw.CaptchaGate(mw.CaptchaScopeEmailVerify), auth.SendEmail)
	Group.POST("email/login", runtimeMw.EmailVerifyMiddleware, auth.EmailLogin)
	// 注册专用路由挂两道：图形验证码的 register 闸门在前（图形码是"人"的证明，先过它能挡批量刷注册），邮箱码校验在后（原样保留）。
	Group.POST("email/register", mw.CaptchaGate(mw.CaptchaScopeRegister), runtimeMw.EmailVerifyMiddleware, auth.RegisterEmail)
	Group.POST("qq", auth.QQLogin)
	// 手机号通道：发码带图形验证码中间件（与邮件发码同一道闸），
	// 登录本身由 handler 内调用阿里云核验，不需要中间件。
	Group.POST("phone/verify", mw.CaptchaGate(mw.CaptchaScopePhoneVerify), auth.SendSms)
	Group.POST("phone", auth.PhoneLogin)
	Group.POST("login", mw.CaptchaGate(mw.CaptchaScopeLogin), auth.PwdLogin)
	Group.POST("refresh", auth.Refresh)
	Group.PUT("password/recovery/email", runtimeMw.EmailVerifyMiddleware, auth.ResetPassword)
	authGroup.PUT("password/renewal/email", auth.UpdatePassword)
	authGroup.POST("password/set", auth.SetPassword)
	// 绑定手机号：登录后操作，号码已通过短信验证（验证码走 phone/verify 那条通道）。
	authGroup.POST("phone/bind", auth.BindPhone)
	authGroup.POST("logout", auth.Logout)
	authGroup.POST("logout/all", auth.LogoutAll)
	authGroup.PUT("email/bind", runtimeMw.EmailVerifyMiddleware, auth.BindEmail)
	authGroup.GET("sessions", auth.Sessions)
	authGroup.DELETE("sessions/:id", auth.DeleteSession)

	profile := userHandler
	authGroup.GET("detail", profile.Detail)
	Group.GET("base", runtimeMw.OptionalAuthMiddleware, profile.BaseInfo)
	// 公开用户搜索：顶栏搜索的「用户」分栏，匿名可读。
	Group.GET("search", profile.SearchUsers)
	authGroup.PUT("info", profile.UpdateInfo)
	adminGroup.PUT("admin/info", mw.CaptureLog(mw.ReqBody), profile.AdminUpdate)
	// 提前解除禁言（票 399）：账号管理页的动作，只给管理员。
	adminGroup.POST("admin/unmute", mw.CaptureLog(mw.ReqBody), profile.AdminUnmute)

	// 登录日志归 analytics 模块（S4.9）。
	analytics := mustAnalyticsHandler()
	adminGroup.GET("login/log", analytics.UserLoginLogList)

	adminGroup.POST("admin", profile.AdminCreate)
	adminGroup.GET("admin/list", profile.AdminList)
}
