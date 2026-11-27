package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	siteapp "myblogx/internal/site/app"
	"myblogx/internal/site/apptest"

	"myblogx/conf"
	confsite "myblogx/conf/site"

	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// 2026-11-28 / ADR-0037 §6.1：`PUT /site/article-review` 的端到端行为。
//
// 这条用例存在的首要理由是**它发现过一个真 bug**：路由上挂了 `mw.BindJson`
// （中间件内部已经 ShouldBindJSON 一次），handler 若再绑一次，第二次读同一个
// Body 拿到 EOF → 每一次保存都返回 1001。单测服务层发现不了这件事，只有把
// 真实路由 + 真实中间件装起来才会红。

// newReviewRouter 装一条最小可用链路：真中间件（CaptureLog + ReqBody + BindJson）
// + 真 handler + 内存 service。
func newReviewRouter(t *testing.T) (*gin.Engine, *siteapp.Service) {
	t.Helper()

	env := apptest.NewEnv(conf.Site{
		SiteInfo: confsite.SiteInfo{Beian: "粤ICP备12345678号"},
		Seo:      confsite.Seo{Keywords: "go,blog", Description: "站点描述"},
	}, conf.AI{Enable: true, TimeoutSec: 60}, "v1.2.3", "")
	env.Service.UpdateSite(conf.Site{
		SiteInfo: confsite.SiteInfo{Beian: "粤ICP备12345678号"},
		Seo:      confsite.Seo{Keywords: "go,blog", Description: "站点描述"},
		Article:  confsite.Article{ScoreEnabled: true, AIComplianceEnabled: true},
	})

	h := NewHandler(env.Service, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 与生产同一条中间件链：审计取 body → 绑定 → handler。
	r.PUT("/api/site/article-review",
		mw.CaptureLog(mw.ReqBody),
		mw.BindJson[ArticleReviewSaveRequest],
		h.UpdateArticleReviewSection)
	return r, env.Service
}

// putReview 发一次保存并返回 (状态码, 解出来的业务码, 原始 body)。
func putReview(t *testing.T, r *gin.Engine, body string) (int, int, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/api/site/article-review", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)

	var envelope struct {
		Code float64 `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	return recorder.Code, int(envelope.Code), recorder.Body.String()
}

// TestUpdateArticleReviewSectionMustNotBindTwice 钉住那个 1001 回归。
func TestUpdateArticleReviewSectionMustNotBindTwice(t *testing.T) {
	t.Parallel()

	r, svc := newReviewRouter(t)
	before := svc.AdminSite()

	status, code, body := putReview(t, r, `{
		"article": {"score_enabled": false, "ai_compliance_enabled": true, "ai_failure_action": "manual"}
	}`)
	if status != http.StatusOK || code != 0 {
		t.Fatalf("保存应成功（若返回 1001 说明 handler 又绑了一次 body）：status=%d code=%d body=%s",
			status, code, body)
	}

	after := svc.AdminSite()
	// 本页字段生效。
	if after.Article.ScoreEnabled {
		t.Fatal("score_enabled=false 应被写入")
	}
	// 评分关 ⇒ 闸门强写 false。
	if after.Article.QualityGate.Enabled {
		t.Fatal("评分关闭时闸门必须落库为 false")
	}
	// **其它七块一个字节都不能变**（这是 article 块类型钉错的经典症状）。
	if after.SiteInfo.Beian != before.SiteInfo.Beian {
		t.Fatalf("site_info 被清零了（article 字段类型错了）：%q → %q",
			before.SiteInfo.Beian, after.SiteInfo.Beian)
	}
	if after.Seo.Description != before.Seo.Description {
		t.Fatalf("seo 被清零了：%q → %q", before.Seo.Description, after.Seo.Description)
	}
}

// TestUpdateArticleReviewSectionOnlyContentFilterKeepsSite 钉住"只传一段时另一段不动"。
func TestUpdateArticleReviewSectionOnlyContentFilterKeepsSite(t *testing.T) {
	t.Parallel()

	r, svc := newReviewRouter(t)
	svc.UpdateContentFilter(conf.ContentFilter{
		Words:      []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		RetainDays: 180,
	})
	before := svc.AdminSite()

	status, code, body := putReview(t, r, `{"content_filter": {"auto_dispose": {"block_action": "draft", "watch_action": "release"}}}`)
	if status != http.StatusOK || code != 0 {
		t.Fatalf("保存应成功：status=%d code=%d body=%s", status, code, body)
	}

	// 处置生效。
	filter := svc.AdminContentFilter()
	if filter.AutoDispose.BlockAction != "draft" {
		t.Fatalf("auto_dispose 应被写入，实际 %+v", filter.AutoDispose)
	}
	// 词表与保留期保持现值（这段归词表页所有）。
	if len(filter.Words) != 1 || filter.RetainDays != 180 {
		t.Fatalf("词表 / 保留期应保持现值，实际 words=%d retain=%d", len(filter.Words), filter.RetainDays)
	}
	// site 段一个字节都没动。
	if svc.AdminSite().Seo.Description != before.Seo.Description {
		t.Fatal("只传 content_filter 时 site 段不该被改动")
	}
}

// TestUpdateArticleReviewSectionRejectsEmptyBody 钉住"两段都不传 → 1001"。
func TestUpdateArticleReviewSectionRejectsEmptyBody(t *testing.T) {
	t.Parallel()

	r, _ := newReviewRouter(t)
	status, code, body := putReview(t, r, `{}`)
	if status == http.StatusOK && code == 0 {
		t.Fatalf("空请求体必须被拒绝，实际成功：%s", body)
	}
	if code == 0 {
		t.Fatalf("应返回非零业务码，实际 body=%s", body)
	}
}

// TestUpdateArticleReviewSectionRejectsEmptyContentFilter 钉住 content_filter 白名单。
func TestUpdateArticleReviewSectionRejectsEmptyContentFilter(t *testing.T) {
	t.Parallel()

	r, _ := newReviewRouter(t)
	status, code, body := putReview(t, r, `{"content_filter": {}}`)
	if status == http.StatusOK && code == 0 {
		t.Fatalf("content_filter 里只有 auto_dispose 可写，空对象必须被拒绝：%s", body)
	}
}
