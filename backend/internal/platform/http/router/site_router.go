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
// 运行时配置各有具名端点（#283 / ADR-0020，第 11 段 content_filter 见 ADR-0032，
// 第 12 段 captcha 见 ADR-0033），
// 参数化 /site/{name} 与 /site/admin/{name} 已整体退场；静态路径不再与通配段争抢。
func SiteRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	app := mustSiteHandler()

	group := r.Group("site")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	group.GET("qq_url", app.QQLoginURL)
	group.GET("seo", app.SEO)
	group.GET("ai_info", app.AIInfo)
	group.GET("sidebar_cards", app.SidebarCards)

	// 登录方式与验证码闸门的公开只读快照（票 481）：这几个布尔位以前搭着
	// 公开的 site 段一起发；拆段之后由它单独给出，免得 mail / sms / oauth / captcha
	// 四段（含密钥）被顺手公开出去。
	group.GET("login_capabilities", app.LoginCapabilities)

	// 公开读：site 与 deco 是唯二对读者开放的段。content_filter 不在其中 ——
	// 词表匿名可读等于把"该绕开哪些词"提前告诉所有人（ADR-0032）。
	group.GET("site", app.PublicSiteSection)
	group.GET("deco", app.PublicDecoSection)

	// 管理读 / 写：每段一一对应具名方法，审计的 TargetID 由 handler 写死段名。
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
		{"deco", app.AdminDecoSection, app.UpdateDecoSection},
		{"image_gc", app.AdminImageGcSection, app.UpdateImageGcSection},
		{"content_filter", app.AdminContentFilterSection, app.UpdateContentFilterSection},
		// 第 12 段 captcha（ADR-0033）：图形验证码的图形与时效参数，只给后台读写。
		{"captcha", app.AdminCaptchaSection, app.UpdateCaptchaSection},
	} {
		adminGroup.GET("admin/"+route.path, route.read)
		adminGroup.PUT(route.path, mw.CaptureLog(mw.ReqBody), route.update)
	}

	// 右栏卡片是独立运营位，不属于运行时配置段注册表。
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
