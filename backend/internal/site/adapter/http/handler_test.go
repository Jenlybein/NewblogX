package http_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
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

// 公开站点配置的形状：不再有站点模式，但有发布/展示三开关（#60 / ADR-0017）。
//
// 站点模式（site_info.mode）是死配置 —— 全仓没有一处读它，留着只会让读者站
// 一直被一个无人消费的字段误导。三个开关在 site.article 段里，随同一份公开
// 配置下发；键名与后端 json tag 一致，前端手写投影照它们绑定。
func TestPublicSiteConfigSwitches(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{Article: siteconf.Article{
		AllowNonAdminPublish: true,
		AllowNonAdminDraft:   true,
		ShowNonAdminArticles: true,
	}}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/site/site", nil)
	c.Set("requestUri", sitehttp.SiteInfoRequest{Name: "site"})
	h.PublicInfo(c)

	body := recorder.Body.String()
	if strings.Contains(body, `"mode"`) {
		t.Fatalf("公开站点配置不应再返回 mode，实际: %s", body)
	}
	for _, key := range []string{"allow_non_admin_publish", "allow_non_admin_draft", "show_non_admin_articles"} {
		if !strings.Contains(body, `"`+key+`":true`) {
			t.Fatalf("公开站点配置应带上 site.article.%s=true，实际: %s", key, body)
		}
	}
}

// 装饰段（deco）走的是既有的三条参数化 name 路由：公开读 GET /site/{name}、
// 管理读 GET /site/admin/{name}、更新 PUT /site/{name}。
// 三条各有一个 switch，漏掉任何一条的表现都是"后台填了、读者看不到"，
// 而编译器和契约都发现不了 —— 只能靠这里钉住。
func TestDecoRoutedThroughNameSwitches(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	if err := env.Runtime.UpdateDeco(conf.Deco{Charm: conf.Charm{Enable: true, LogoSize: 60}}); err != nil {
		t.Fatalf("预置装饰配置失败: %v", err)
	}
	h := sitehttp.NewHandler(env.Service, nil)

	call := func(fn func(*gin.Context), method, name, body string) string {
		handler := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(handler)
		c.Request = httptest.NewRequest(method, "/site/"+name, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("requestUri", sitehttp.SiteInfoRequest{Name: name})
		fn(c)
		return handler.Body.String()
	}

	t.Run("公开读带上 deco 分支", func(t *testing.T) {
		body := call(h.PublicInfo, "GET", "deco", "")
		if strings.Contains(body, "站点信息不存在") {
			t.Fatalf("GET /site/deco 应命中 deco 分支，实际: %s", body)
		}
		if !strings.Contains(body, `"logo_size":60`) {
			t.Fatalf("公开读应返回当前装饰配置，实际: %s", body)
		}
	})

	t.Run("管理读带上 deco 分支", func(t *testing.T) {
		body := call(h.AdminInfo, "GET", "deco", "")
		if strings.Contains(body, "站点信息不存在") {
			t.Fatalf("GET /site/admin/deco 应命中 deco 分支，实际: %s", body)
		}
		if !strings.Contains(body, `"logo_size":60`) {
			t.Fatalf("管理读应返回当前装饰配置，实际: %s", body)
		}
	})

	t.Run("更新带上 deco 分支且落库前钳位", func(t *testing.T) {
		body := call(h.UpdateConfig, "PUT", "deco", `{"charm":{"enable":false,"logo_size":9999}}`)
		if strings.Contains(body, "站点信息不存在") {
			t.Fatalf("PUT /site/deco 应命中 deco 分支，实际: %s", body)
		}
		got := env.Runtime.Deco()
		if got.Charm.Enable {
			t.Fatal("开关应被写成 false")
		}
		if got.Charm.LogoSize != conf.CharmLogoSizeMax {
			t.Fatalf("9999 应被钳到 %v，实际 %v", conf.CharmLogoSizeMax, got.Charm.LogoSize)
		}
	})

	t.Run("分数 segments 不再 400：绑定后由 Normalize 四舍五入成整数", func(t *testing.T) {
		// 回归：Segments 曾是 Go 的 int，`{"segments":10.6}` 在 JSON 绑定阶段
		// 就失败并返回 400，管理员根本到不了 Normalize —— 违反"永不报错"。
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("PUT", "/site/deco", strings.NewReader(`{"charm":{"segments":10.6}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("requestUri", sitehttp.SiteInfoRequest{Name: "deco"})
		h.UpdateConfig(c)

		if recorder.Code != 200 {
			t.Fatalf("分数 segments 不应被拒绝，HTTP 状态 %d，body: %s", recorder.Code, recorder.Body.String())
		}
		got := env.Runtime.Deco()
		if got.Charm.Segments != 11 {
			t.Fatalf("10.6 节应四舍五入为 11，实际 %v", got.Charm.Segments)
		}
	})
}
