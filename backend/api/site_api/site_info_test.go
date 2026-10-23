package site_api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/api/site_api"
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/models"
	"myblogx/service/site_service"
	"myblogx/test/testutil"

	"github.com/gin-gonic/gin"
)

func newSiteCtx(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func readSiteCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	return int(body["code"].(float64))
}

func setupSiteApiEnv(t *testing.T) (site_api.SiteApi, *site_service.RuntimeConfigService) {
	t.Helper()
	db := testutil.SetupSQLite(t, &models.RuntimeSiteConfigModel{})
	cfg := testutil.SetConfig(&conf.Config{
		Site: conf.Site{
			SiteInfo: confsite.SiteInfo{
				Title: "技术博客",
				Logo:  "/logo.png",
			},
			Project: confsite.Project{
				Title: "项目标题",
				Icon:  "/favicon.ico",
			},
			Seo: confsite.Seo{
				Keywords:    "go,blog",
				Description: "站点描述",
			},
		},
		QQ: conf.QQ{
			AppID:    "app-id",
			AppKey:   "app-key-origin",
			Redirect: "https://example.com/callback",
		},
		AI: conf.AI{
			Enable:    true,
			SecretKey: "ai-secret-origin",
			BaseURL:   "https://ai.example.com/v1/chat/completions",
			ChatModel: "gpt-test",
			Nickname:  "AI 助手",
			Avatar:    "/ai.png",
			Abstract:  "你好",
		},
	})

	runtimeSvc := site_service.NewRuntimeConfigService(cfg.Site, cfg.AI, testutil.Logger(), db, "")
	if err := runtimeSvc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化运行时站点配置失败: %v", err)
	}

	api := site_api.New(site_api.Deps{
		Version:     testutil.Version(),
		QQ:          cfg.QQ,
		RuntimeSite: runtimeSvc,
		Redis:       testutil.Redis(),
		Logger:      testutil.Logger(),
	})
	return api, runtimeSvc
}

func TestSiteInfoViews(t *testing.T) {
	api, _ := setupSiteApiEnv(t)

	t.Run("QQ登录地址", func(t *testing.T) {
		c, w := newSiteCtx(httptest.NewRequest(http.MethodGet, "/site/qq_url", nil))
		api.SiteInfoQQView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("QQ 地址接口应成功, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "graph.qq.com") {
			t.Fatalf("QQ 地址返回异常: %s", w.Body.String())
		}
	})

	t.Run("站点信息", func(t *testing.T) {
		c, w := newSiteCtx(httptest.NewRequest(http.MethodGet, "/site/site", nil))
		c.Set("requestUri", site_api.SiteInfoRequest{Name: "site"})
		api.SiteInfoView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("站点信息接口应成功, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), testutil.Version()) {
			t.Fatalf("站点信息应包含版本号, body=%s", w.Body.String())
		}
	})

	t.Run("SEO 信息", func(t *testing.T) {
		c, w := newSiteCtx(httptest.NewRequest(http.MethodGet, "/site/seo", nil))
		api.SiteSEOView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("SEO 接口应成功, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "项目标题") || !strings.Contains(w.Body.String(), "站点描述") {
			t.Fatalf("SEO 返回异常: %s", w.Body.String())
		}
	})

	t.Run("AI 信息", func(t *testing.T) {
		c, w := newSiteCtx(httptest.NewRequest(http.MethodGet, "/site/ai_info", nil))
		api.SiteInfoAIView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("AI 信息接口应成功, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "AI 助手") {
			t.Fatalf("AI 信息返回异常: %s", w.Body.String())
		}
	})
}

func TestSiteUpdateView(t *testing.T) {
	api, runtimeSvc := setupSiteApiEnv(t)

	t.Run("未知配置名", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/site/unknown", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		c, w := newSiteCtx(req)
		c.Set("requestUri", site_api.SiteInfoRequest{Name: "unknown"})
		api.SiteUpdateView(c)
		if code := readSiteCode(t, w); code == 0 {
			t.Fatalf("未知配置名应失败, body=%s", w.Body.String())
		}
	})

	t.Run("AI 敏感字段占位符保留原值", func(t *testing.T) {
		body := `{"enable":true,"secret":"******","base_url":"https://new-ai.example.com","chat_model":"gpt-4.1","nickname":"新助手","avatar":"/new-ai.png","abstract":"新的简介"}`
		req := httptest.NewRequest(http.MethodPost, "/site/ai", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		c, w := newSiteCtx(req)
		c.Set("requestUri", site_api.SiteInfoRequest{Name: "ai"})
		api.SiteUpdateView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("ai 更新应成功, body=%s", w.Body.String())
		}
		current := runtimeSvc.GetRuntimeAI()
		if current.SecretKey != "ai-secret-origin" {
			t.Fatalf("占位符应保留原 ai secret, got=%s", current.SecretKey)
		}
		if current.BaseURL != "https://new-ai.example.com" {
			t.Fatalf("AI base_url 未更新, got=%s", current.BaseURL)
		}
	})

	t.Run("站点运行时配置写入数据库", func(t *testing.T) {
		body := `{"site_info":{"title":"新站点","logo":"/new-logo.png","beian":"粤ICP备0001号","mode":1},"project":{"title":"新项目","icon":"/new.ico","web_path":"uploads/index.html"},"seo":{"keywords":"k1,k2","description":"新的描述"},"about":{"site_date":"","qq":"","wechat":"","gitee":"","bilibili":"","github":""},"login":{"qq_login":true,"username_pwd_login":true,"email_login":true,"captcha":false,"email_code_timeout":10,"login_fail_window_minute":15,"login_fail_user_max":5,"login_fail_ip_max":20,"email_send_window_second":60,"email_send_per_email_max":1,"email_send_per_ip_max":10},"index_right":{"list":[]},"article":{"skip_examining":true},"comment":{"skip_examining":true}}`
		req := httptest.NewRequest(http.MethodPost, "/site/site", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		c, w := newSiteCtx(req)
		c.Set("requestUri", site_api.SiteInfoRequest{Name: "site"})
		api.SiteUpdateView(c)
		if code := readSiteCode(t, w); code != 0 {
			t.Fatalf("site 更新应成功, body=%s", w.Body.String())
		}
		if got := runtimeSvc.GetRuntimeSite().SiteInfo.Title; got != "新站点" {
			t.Fatalf("站点标题未更新, got=%s", got)
		}
		if got := runtimeSvc.GetRuntimeSite().Seo.Description; got != "新的描述" {
			t.Fatalf("运行时站点配置未更新, got=%s", got)
		}
	})
}
