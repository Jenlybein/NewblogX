package router

import (
	"myblogx/api"
	"myblogx/api/user_api/auth_api"
	"myblogx/api/user_api/log_api"
	"myblogx/api/user_api/profile_api"
	"myblogx/api/user_api/user_man_api"
	mw "myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

func UserRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	Group := r.Group("users")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	auth := appContainer.UserApi.AuthApi
	Group.POST("email/verify", runtimeMw.CaptchaMiddleware, mw.BindJson[auth_api.SendEmailRequest], auth.SendEmailView)
	Group.POST("email/login", runtimeMw.EmailVerifyMiddleware, auth.EmailLoginView)
	Group.POST("email/register", runtimeMw.EmailVerifyMiddleware, mw.BindJson[auth_api.RegisterEmailRequest], auth.RegisterEmailView)
	Group.POST("qq", mw.BindJson[auth_api.QQLoginRequest], auth.QQLoginView)
	Group.POST("login", runtimeMw.CaptchaMiddleware, mw.BindJson[auth_api.PwdLoginRequest], auth.PwdLoginView)
	Group.POST("refresh", auth.RefreshTokenView)
	Group.PUT("password/recovery/email", runtimeMw.EmailVerifyMiddleware, mw.BindJson[auth_api.ResetPasswordRequest], auth.ResetPwdByEmailView)
	authGroup.PUT("password/renewal/email", mw.BindJson[auth_api.UpdatePasswordRequest], auth.UpdatePwdByEmailView)
	authGroup.POST("logout", auth.UserLogoutView)
	authGroup.POST("logout/all", auth.UserLogoutAllView)
	authGroup.PUT("email/bind", runtimeMw.EmailVerifyMiddleware, auth.BindEmailView)
	authGroup.GET("sessions", mw.BindQuery[auth_api.UserSessionListRequest], auth.UserSessionListView)
	authGroup.DELETE("sessions/:id", mw.BindUri[models.IDRequest], auth.UserSessionDeleteView)

	profile := appContainer.UserApi.ProfileApi
	authGroup.GET("detail", profile.UserDetailView)
	Group.GET("base", runtimeMw.OptionalAuthMiddleware, mw.BindQuery[models.IDRequest], profile.UserBaseInfoView)
	authGroup.PUT("info", mw.BindJson[profile_api.UserInfoUpdateRequest], profile.UserInfoUpdateView)
	adminGroup.PUT("admin/info", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[profile_api.AdminUserInfoUpdateRequest], profile.AdminUserInfoUpdateView)

	log := appContainer.UserApi.LogApi
	adminGroup.GET("login/log", mw.BindQuery[log_api.UserLoginListRequest], log.UserLoginLogList)

	man := appContainer.UserApi.UserManApi
	adminGroup.POST("admin", mw.CaptureLog(mw.ReqHeader), mw.BindJson[user_man_api.AdminUserCreateRequest], man.AdminUserCreateView)
	adminGroup.GET("admin/list", mw.BindQuery[user_man_api.UserListRequest], man.UserListView)
}
