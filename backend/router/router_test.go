package router_test

import (
	analyticsch "myblogx/internal/analytics/adapter/clickhouse"
	analyticshttp "myblogx/internal/analytics/adapter/http"
	analyticsmysql "myblogx/internal/analytics/adapter/mysql"
	articlehttp "myblogx/internal/article/adapter/http"
	authhttp "myblogx/internal/auth/adapter/http"
	authapptest "myblogx/internal/auth/apptest"
	mediahttp "myblogx/internal/media/adapter/http"
	mediaapptest "myblogx/internal/media/apptest"
	notificationhttp "myblogx/internal/notification/adapter/http"
	notificationtest "myblogx/internal/notification/apptest"
	"myblogx/internal/platform/captcha"
	mw "myblogx/internal/platform/http/middleware"
	searchhttp "myblogx/internal/search/adapter/http"
	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"
	userhttp "myblogx/internal/user/adapter/http"
	userapptest "myblogx/internal/user/apptest"
	"myblogx/router"
	"myblogx/test/testutil"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

func TestRegisterAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	router.HealthRouter(r)
	api := r.Group("/api")
	runtimeMw := mw.Runtime{}

	router.SetSiteHandler(sitehttp.NewHandler(sitetest.NewDefaultService()))
	router.SiteRouter(api, runtimeMw)
	router.SetAnalyticsHandler(analyticshttp.NewHandler(analyticsch.Options{}, analyticsmysql.NewStats(testutil.DB()), nil, nil))
	router.LogRouter(api, runtimeMw)
	router.SetImageHandler(mediahttp.NewHandler(mediaapptest.NewDefaultService()))
	router.ImageRouter(api, runtimeMw)
	router.BannerRouter(api, runtimeMw)
	router.SetCaptchaHandler(captcha.NewHandler(captchaSettings{true}, base64Captcha.DefaultMemStore))
	router.CaptchaRouter(api)
	// 认证路由来自 internal/auth：这里用全空替身，仅验证路由表。
	router.SetAuthHandler(authhttp.NewHandler(authapptest.NewRepos(time.Now()).Service(), nil, nil, nil, false, 0))
	router.SetUserHandler(userhttp.NewHandler(userapptest.NewRepos(time.Now()).Service()))
	router.UserRouter(api, runtimeMw)
	// 旧 article_api 已删除，路由只来自新 handler；这里用空依赖的处理器
	// 仅验证路由表，不发起真实请求。
	router.SetArticleHandler(articlehttp.NewHandler(nil, nil, nil, nil, nil))
	router.ArticleRouter(api, runtimeMw)
	router.SetSearchHandler(searchhttp.NewHandler(nil, nil, nil, "", nil))
	router.SearchRouter(api, runtimeMw)
	router.SetNotificationHandler(notificationhttp.NewHandler(notificationtest.NewDefaultService()))
	router.SitemsgRouter(api, runtimeMw)
	router.GlobalNotifRouter(api, runtimeMw)

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

// captchaSettings 是验证码开关替身（路由表测试只关心路由是否注册成功）。
type captchaSettings struct{ enabled bool }

func (c captchaSettings) CaptchaEnabled() bool { return c.enabled }
