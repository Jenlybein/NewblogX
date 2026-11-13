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
//
// 九段运行时配置各有具名端点（#283 / ADR-0020），参数化 /site/{name} 与
// /site/admin/{name} 已整体退场；静态路径不再与通配段争抢。
func SiteRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustSiteHandler()

	group := r.Group("site")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	group.GET("qq_url", app.QQLoginURL)
	group.GET("seo", app.SEO)
	group.GET("ai_info", app.AIInfo)
	group.GET("sidebar_cards", app.SidebarCards)

	// 公开读：site 与 deco 是九段里唯二对读者开放的段。
	group.GET("site", app.PublicSiteSection)
	group.GET("deco", app.PublicDecoSection)

	// 管理读 / 写：九段一一对应具名方法，审计的 TargetID 由 handler 写死段名。
	for _, route := range []struct {
		path   string
		read   gin.HandlerFunc
		update gin.HandlerFunc
	}{
		{"site", app.AdminSiteSection, app.UpdateSiteSection},
		{"ai", app.AdminAiSection, app.UpdateAiSection},
		{"mail", app.AdminMailSection, app.UpdateMailSection},
		{"storage", app.AdminStorageSection, app.UpdateStorageSection},
		{"oauth", app.AdminOauthSection, app.UpdateOauthSection},
		{"sms", app.AdminSmsSection, app.UpdateSmsSection},
		{"log", app.AdminLogSection, app.UpdateLogSection},
		{"upload", app.AdminUploadSection, app.UpdateUploadSection},
		{"deco", app.AdminDecoSection, app.UpdateDecoSection},
		{"image_gc", app.AdminImageGcSection, app.UpdateImageGcSection},
	} {
		adminGroup.GET("admin/"+route.path, route.read)
		adminGroup.PUT(route.path, mw.CaptureLog(mw.ReqBody), route.update)
	}

	// 右栏卡片是独立运营位，不属于九段注册表。
	adminGroup.GET("admin/sidebar_cards", app.AdminSidebarCards)
	adminGroup.PUT("admin/sidebar_cards", mw.CaptureLog(mw.ReqBody), mw.BindJson[sitehttp.SidebarCardsSaveRequest], app.UpdateSidebarCards)
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
