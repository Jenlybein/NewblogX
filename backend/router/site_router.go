package router

import (
	mw "myblogx/internal/platform/http/middleware"
	sitehttp "myblogx/internal/site/adapter/http"

	"github.com/gin-gonic/gin"
)

// siteHandler 由 app.WireSite 在启动时注入。
//
// 站点配置与轮播图共用一个处理器。
var siteHandler *sitehttp.Handler

// SetSiteHandler 注入站点处理器。
func SetSiteHandler(h *sitehttp.Handler) { siteHandler = h }

// mustSiteHandler 取处理器，未注入直接 panic（与 article 一致）。
func mustSiteHandler() *sitehttp.Handler {
	if siteHandler == nil {
		panic("router: 站点处理器未注入（应在 app.WireSite 中调用 SetSiteHandler）")
	}
	return siteHandler
}

// SiteRouter 注册站点配置路由。
func SiteRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustSiteHandler()

	group := r.Group("site")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	// 静态路径必须先于 :name 注册，否则会被通配吃掉。
	group.GET("qq_url", app.QQLoginURL)
	group.GET("seo", app.SEO)
	group.GET("ai_info", app.AIInfo)
	group.GET(":name", mw.BindUri[sitehttp.SiteInfoRequest], app.PublicInfo)

	adminGroup.GET("admin/:name", mw.BindUri[sitehttp.SiteInfoRequest], app.AdminInfo)
	adminGroup.PUT(":name", mw.CaptureLog(mw.ReqBody), mw.BindUri[sitehttp.SiteInfoRequest], app.UpdateConfig)
}

// BannerRouter 注册轮播图路由。
func BannerRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustSiteHandler()

	group := r.Group("banners")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	group.GET("", mw.BindQuery[sitehttp.BannerListRequest], app.BannerList)
	adminGroup.POST("", mw.CaptureLog(mw.ReqBody), mw.BindJson[sitehttp.BannerCreateRequest], app.BannerCreate)
	adminGroup.PUT(":id", mw.CaptureLog(mw.ReqBody), mw.BindUri[sitehttp.IDRequest], mw.BindJson[sitehttp.BannerCreateRequest], app.BannerUpdate)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody), mw.BindJson[sitehttp.IDListRequest], app.BannerRemove)
}
