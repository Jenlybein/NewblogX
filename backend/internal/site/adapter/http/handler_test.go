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

// 站点访问量埋点的回归测试：公开读 site 段时触发一次计数，deco 段不计数。
func TestPublicSiteRecordsVisit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var visits int
	h := sitehttp.NewHandler(sitetest.NewDefaultService(), func(_, _ string) { visits++ })

	newCtx := func(path string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", path, nil)
		return c
	}

	t.Run("读公开站点配置记一次访问", func(t *testing.T) {
		visits = 0
		h.PublicSiteSection(newCtx("/site/site"))
		if visits != 1 {
			t.Fatalf("读取 /site/site 应累计 1 次访问，实际 %d", visits)
		}
	})

	t.Run("公开装饰配置不计数", func(t *testing.T) {
		visits = 0
		h.PublicDecoSection(newCtx("/site/deco"))
		if visits != 0 {
			t.Fatalf("读取 /site/deco 不该计数，实际 %d", visits)
		}
	})

	t.Run("未注入计数器时不应 panic", func(t *testing.T) {
		zero := sitehttp.NewHandler(sitetest.NewDefaultService(), nil)
		zero.PublicSiteSection(newCtx("/site/site"))
	})
}

// 公开站点配置的形状：不再有站点模式，但有发布/展示三开关（#60 / ADR-0015）。
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
	h.PublicSiteSection(c)

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

// deco 段走具名端点：公开读 / 管理读 / 更新三个出入口都不再靠 name 分派。
func TestDecoNamedSectionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	if err := env.Runtime.UpdateDeco(conf.Deco{Charm: conf.Charm{Enable: true, LogoSize: 60}}); err != nil {
		t.Fatalf("预置装饰配置失败: %v", err)
	}
	h := sitehttp.NewHandler(env.Service, nil)

	call := func(fn func(*gin.Context), method, path, body string) string {
		handler := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(handler)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		fn(c)
		return handler.Body.String()
	}

	t.Run("公开读返回当前装饰配置", func(t *testing.T) {
		body := call(h.PublicDecoSection, "GET", "/site/deco", "")
		if !strings.Contains(body, `"logo_size":60`) {
			t.Fatalf("公开读应返回当前装饰配置，实际: %s", body)
		}
	})

	t.Run("管理读返回当前装饰配置", func(t *testing.T) {
		body := call(h.AdminDecoSection, "GET", "/site/admin/deco", "")
		if !strings.Contains(body, `"logo_size":60`) {
			t.Fatalf("管理读应返回当前装饰配置，实际: %s", body)
		}
	})

	t.Run("更新落库前钳位", func(t *testing.T) {
		body := call(h.UpdateDecoSection, "PUT", "/site/deco", `{"charm":{"enable":false,"logo_size":9999}}`)
		if strings.Contains(body, "站点信息不存在") {
			t.Fatalf("PUT /site/deco 应命中具名更新，实际: %s", body)
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
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("PUT", "/site/deco", strings.NewReader(`{"charm":{"segments":10.6}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.UpdateDecoSection(c)

		if recorder.Code != 200 {
			t.Fatalf("分数 segments 不应被拒绝，HTTP 状态 %d，body: %s", recorder.Code, recorder.Body.String())
		}
		got := env.Runtime.Deco()
		if got.Charm.Segments != 11 {
			t.Fatalf("10.6 节应四舍五入为 11，实际 %v", got.Charm.Segments)
		}
	})
}

// image_gc 是第九段：具名读 / 写出入口必须真的接进运行时配置（#283 前无 HTTP 出入口）。
func TestImageGCNamedSectionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("PUT", "/site/image_gc", strings.NewReader(`{"enabled":true,"grace_days":7,"batch_limit":20}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateImageGcSection(c)
	if recorder.Code != 200 {
		t.Fatalf("PUT /site/image_gc 应成功，状态 %d，body: %s", recorder.Code, recorder.Body.String())
	}

	readRecorder := httptest.NewRecorder()
	readCtx, _ := gin.CreateTestContext(readRecorder)
	readCtx.Request = httptest.NewRequest("GET", "/site/admin/image_gc", nil)
	h.AdminImageGcSection(readCtx)
	if !strings.Contains(readRecorder.Body.String(), `"grace_days":7`) || !strings.Contains(readRecorder.Body.String(), `"batch_limit":20`) {
		t.Fatalf("管理读应返回刚写入的 image_gc，实际: %s", readRecorder.Body.String())
	}
}
