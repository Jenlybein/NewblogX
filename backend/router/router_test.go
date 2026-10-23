package router_test

import (
	api2 "myblogx/api"
	mw "myblogx/middleware"
	"myblogx/router"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	router.HealthRouter(r)
	api := r.Group("/api")
	app := api2.New(api2.Deps{})
	runtimeMw := mw.Runtime{}

	router.SiteRouter(api, app, runtimeMw)
	router.LogRouter(api, app, runtimeMw)
	router.ImageRouter(api, app, runtimeMw)
	router.BannerRouter(api, app, runtimeMw)
	router.CaptchaRouter(api, app, runtimeMw)
	router.UserRouter(api, app, runtimeMw)
	router.ArticleRouter(api, app, runtimeMw)
	router.SearchRouter(api, app, runtimeMw)
	router.SitemsgRouter(api, app, runtimeMw)
	router.GlobalNotifRouter(api, app, runtimeMw)

	routes := r.Routes()
	if len(routes) == 0 {
		t.Fatal("路由不应为空")
	}

	routeSet := make(map[string]bool)
	for _, rt := range routes {
		routeSet[rt.Method+" "+rt.Path] = true
	}

	expect := []string{
		"GET /health",
		"GET /api/site/qq_url",
		"GET /api/site/:name",
		"GET /api/imagecaptcha",
		"GET /api/logs/runtime",
		"GET /api/logs/login",
		"GET /api/logs/action",
		"POST /api/images/upload-tasks",
		"POST /api/images/upload-tasks/complete",
		"POST /api/images/qiniu/callback",
		"GET /api/banners",
		"POST /api/users/login",
		"GET /api/users/base",
		"GET /api/users/sessions",
		"DELETE /api/users/sessions/:id",
		"POST /api/users/admin",
		"GET /api/articles/author_info",
		"GET /api/search/articles",
		"GET /api/sitemsg",
		"GET /api/global_notif",
		"POST /api/global_notif/read",
		"DELETE /api/global_notif/user",
		"POST /api/global_notif",
		"DELETE /api/global_notif",
	}
	for _, e := range expect {
		if !routeSet[e] {
			t.Fatalf("缺少路由: %s", e)
		}
	}
}
