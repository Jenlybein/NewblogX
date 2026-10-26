package http_test

import (
	"net/http/httptest"
	"testing"

	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"

	"github.com/gin-gonic/gin"
)

// 站点访问量埋点的回归测试。
//
// 背景：迁移到 internal/site 时把 `redis_site.SetFlow` 那一行漏了，
// 于是 `/site/site` 再也不会累计访问量，而管理端首页的 flow 一直在读它
// （永远是 0）。这个缺陷编译、vet、单测、契约、archcheck 全发现不了——
// 唯一能钉住它的就是"读公开配置会触发一次计数"这条断言。
func TestPublicSiteRecordsVisit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var visits int
	h := sitehttp.NewHandler(sitetest.NewDefaultService(), func() { visits++ })

	newCtx := func(name string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/site/"+name, nil)
		c.Set("requestUri", sitehttp.SiteInfoRequest{Name: name})
		return c
	}

	t.Run("读公开配置记一次访问", func(t *testing.T) {
		visits = 0
		h.PublicInfo(newCtx("site"))
		if visits != 1 {
			t.Fatalf("读取 /site/site 应累计 1 次访问，实际 %d", visits)
		}
	})

	t.Run("其他配置块不计数", func(t *testing.T) {
		visits = 0
		h.PublicInfo(newCtx("seo"))
		if visits != 0 {
			t.Fatalf("读取 /site/seo 不该计数，实际 %d", visits)
		}
	})

	t.Run("未注入计数器时不应 panic", func(t *testing.T) {
		// 路由表测试用的是零值 Handler（没有任何依赖），这条路径必须安全。
		zero := sitehttp.NewHandler(sitetest.NewDefaultService(), nil)
		zero.PublicInfo(newCtx("site"))
	})
}
