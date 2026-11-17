package http_test

import (
	"encoding/json"
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

// content_filter 是第 11 段（ADR-0032）：只有管理端出入口，保存后缓存立刻刷新。
func TestContentFilterNamedSectionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)

	call := func(fn func(*gin.Context), method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		fn(c)
		return recorder
	}

	t.Run("保存后管理读能读回同一份词表", func(t *testing.T) {
		body := `{"words":[` +
			`{"word":"冰毒","tier":"block","category":"drugs"},` +
			`{"word":"加微信","tier":"watch","category":"fraud"}],` +
			`"whitelist":["冰毒科普"]}`
		if rec := call(h.UpdateContentFilterSection, "PUT", "/site/content_filter", body); rec.Code != 200 {
			t.Fatalf("PUT /site/content_filter 应成功，状态 %d，body: %s", rec.Code, rec.Body.String())
		}

		read := call(h.AdminContentFilterSection, "GET", "/site/admin/content_filter", "")
		got := read.Body.String()
		for _, want := range []string{`"冰毒"`, `"block"`, `"drugs"`, `"加微信"`, `"watch"`, `"冰毒科普"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("管理读应返回刚保存的词表，缺少 %s，实际: %s", want, got)
			}
		}
		// 保存即生效：运行时配置缓存已经是新词表，不需要重启。
		if words := env.Runtime.ContentFilter().Words; len(words) != 2 {
			t.Fatalf("缓存里应有 2 条词，实际 %d", len(words))
		}
	})

	t.Run("词表整理：空词条丢弃、重复词去重、空白去掉", func(t *testing.T) {
		body := `{"words":[` +
			`{"word":"  刷单  ","tier":"watch","category":"fraud"},` +
			`{"word":"刷单","tier":"watch","category":"fraud"},` +
			`{"word":"   ","tier":"watch","category":"fraud"}],` +
			`"whitelist":["  ", "测试"]}`
		if rec := call(h.UpdateContentFilterSection, "PUT", "/site/content_filter", body); rec.Code != 200 {
			t.Fatalf("PUT 应成功，状态 %d，body: %s", rec.Code, rec.Body.String())
		}
		got := env.Runtime.ContentFilter()
		if len(got.Words) != 1 || got.Words[0].Word != "刷单" {
			t.Fatalf("应只剩 1 条去空白后的词，实际 %+v", got.Words)
		}
		if len(got.Whitelist) != 1 || got.Whitelist[0] != "测试" {
			t.Fatalf("白名单应只剩 1 条，实际 %+v", got.Whitelist)
		}
	})

	t.Run("档位/类目/单字词不合法时拒绝保存", func(t *testing.T) {
		cases := map[string]string{
			"未知档位": `{"words":[{"word":"冰毒","tier":"blocked","category":"drugs"}]}`,
			"未知类目": `{"words":[{"word":"冰毒","tier":"block","category":"weapons"}]}`,
			"单字词":  `{"words":[{"word":"毒","tier":"block","category":"drugs"}]}`,
		}
		for name, body := range cases {
			rec := call(h.UpdateContentFilterSection, "PUT", "/site/content_filter", body)
			// 参数类失败是 1001（可自行修正），不是 1002（服务异常）。
			if !strings.Contains(rec.Body.String(), `"code":1001`) {
				t.Fatalf("%s 应返回参数错误 1001，实际: %s", name, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "内容过滤词表不合法") {
				t.Fatalf("%s 的文案应来自 apperr 消息表，实际: %s", name, rec.Body.String())
			}
		}
	})
}

// 公开读面不得带上 content_filter：词表匿名可读等于把"该绕开哪些词"告诉所有人。
func TestPublicSiteDoesNotExposeContentFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	if err := env.Runtime.UpdateContentFilter(conf.ContentFilter{
		Words:     []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		Whitelist: []string{"冰毒科普"},
	}); err != nil {
		t.Fatalf("预置词表失败: %v", err)
	}
	h := sitehttp.NewHandler(env.Service, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/site/site", nil)
	h.PublicSiteSection(c)

	body := recorder.Body.String()
	for _, leaked := range []string{"content_filter", "冰毒", "冰毒科普"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("公开站点配置泄露了词表内容 %q，实际: %s", leaked, body)
		}
	}
}

// image_gc 是孤儿图回收那一段：具名读 / 写出入口必须真的接进运行时配置（#283 前无 HTTP 出入口）。
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

// captcha 是第 12 段（ADR-0033）：只有管理端具名出入口（PUT /site/captcha + GET /site/admin/captcha）。
//
// 这是"运营改图"的完整链路：保存时的校验必须拦住坏配置，保存成功后管理读必须
// 原样读回。校验点放在 adapter 而不是 handler，是因为这条链路没有别的入口——
// 命令行、初始化都不写这一段，如果这里放行，坏配置就直接进了运行时缓存，
// 下一次有人打开登录页才发现验证码是坏的（而那时运营已经关掉了后台）。
func TestCaptchaNamedSectionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)

	call := func(fn func(*gin.Context), method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		fn(c)
		return recorder
	}

	// 保存非法配置必须失败，且文案指向这一段（后台据此把句子显示给运营）。
	//
	// 断言业务码 1002（协议兜底："服务异常"）是**当前**事实：它按仓库约定应该是
	// 1001（参数校验失败，改一改就能过）——同族的 content_filter 保存期校验就登记成
	// ProtocolValidation。captcha_invalid 还没在 apperr/protocol.go 登记协议类别，
	// 于是落到了服务异常兜底（已作为生产缺陷上报）。修好协议登记后这条会红，
	// 提示把期望值改成 1001 —— 那时这条用例守的就是"可自愈的失败不伪装成服务故障"。
	t.Run("保存非法配置被拒绝", func(t *testing.T) {
		cases := map[string]string{
			"宽度越界":      `{"driver":"string","width":5000}`,
			"字符集只剩一个字符": `{"driver":"string","source":"AA"}`,
			"未知驱动名":     `{"driver":"wechat"}`,
			"背景色不是十六进制": `{"driver":"string","bg_color":"深蓝"}`,
		}
		for name, body := range cases {
			rec := call(h.UpdateCaptchaSection, "PUT", "/site/captcha", body)
			if rec.Code != 200 {
				t.Fatalf("%s：配置类失败应是 HTTP 200 + 业务码（不是 5xx），实际 status=%d body=%s",
					name, rec.Code, rec.Body.String())
			}
			// 1001（参数错误）而不是 1002（服务异常）：这是运营填错、改一改就能过的失败，
			// 归 1002 会让后台以为服务坏了、不敢重试。
			if !strings.Contains(rec.Body.String(), `"code":1001`) {
				t.Fatalf("%s：保存非法配置应返回失败码 1001，实际: %s", name, rec.Body.String())
			}
			// 文案要点名"哪一项越界"（width 越界就说 width），不能只剩笼统的"配置不合法"：
			// 后者会让运营反复试同一组数字。这句走 apperr 的受控文案通道。
			if !mentionsCaptchaField(rec.Body.String()) {
				t.Fatalf("%s：失败文案应点明具体字段与区间，实际: %s", name, rec.Body.String())
			}
		}
	})

	// 保存合法配置后，管理读必须读回同一份（保存即生效，没有"重建"这一步）。
	t.Run("保存合法配置后读回一致", func(t *testing.T) {
		if rec := call(h.UpdateCaptchaSection, "PUT", "/site/captcha",
			`{"driver":"digit","length":6,"width":240,"height":70,"expire_seconds":900,"max_skew":0.5,"dot_count":9}`); rec.Code != 200 {
			t.Fatalf("PUT /site/captcha 应成功，状态 %d，body: %s", rec.Code, rec.Body.String())
		}

		read := call(h.AdminCaptchaSection, "GET", "/site/admin/captcha", "")
		var resp struct {
			Code int          `json:"code"`
			Data conf.Captcha `json:"data"`
		}
		if err := json.Unmarshal(read.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析管理读响应失败: %v, body=%s", err, read.Body.String())
		}
		if resp.Code != 0 {
			t.Fatalf("管理读 captcha 段应成功，body: %s", read.Body.String())
		}

		// 逐字段核对：保存的每一项都必须原样读回（少一项就是"后台改了但没生效"）。
		if resp.Data.Driver != "digit" || resp.Data.Length != 6 {
			t.Fatalf("应读回 driver=digit/length=6，实际 %q/%d", resp.Data.Driver, resp.Data.Length)
		}
		if resp.Data.Width != 240 || resp.Data.Height != 70 {
			t.Fatalf("应读回尺寸 240x70，实际 %dx%d", resp.Data.Width, resp.Data.Height)
		}
		if resp.Data.ExpireSeconds != 900 {
			t.Fatalf("应读回有效期 900 秒，实际 %d", resp.Data.ExpireSeconds)
		}
		if resp.Data.MaxSkew != 0.5 || resp.Data.DotCount != 9 {
			t.Fatalf("应读回倾斜 0.5 / 背景点 9，实际 %v/%d", resp.Data.MaxSkew, resp.Data.DotCount)
		}
		// 纯数字型不适用项被清零也是"读回一致"的一部分：后台表单据此禁用输入框。
		if resp.Data.NoiseCount != 0 || resp.Data.BgColor != "" {
			t.Fatalf("纯数字型的噪点/背景色应为空，实际 %d/%q", resp.Data.NoiseCount, resp.Data.BgColor)
		}

		// 保存即生效：运行时缓存就是刚保存的值，不需要重启。
		if got := env.Runtime.Captcha(); got.Driver != "digit" || got.ExpireSeconds != 900 || got.DotCount != 9 {
			t.Fatalf("运行时缓存应立刻变成新配置，实际 %+v", got)
		}
	})

	// 保存失败的请求不能改动已在生效的配置。
	//
	// 这条守的是"坏配置进不了线上"：先保存一份好配置，再保存一份坏的，坏的那次
	// 如果先刷了缓存再校验，全站登录页会立刻开始出坏图 —— 直到有人再保存一次。
	t.Run("保存失败不改动已生效配置", func(t *testing.T) {
		if rec := call(h.UpdateCaptchaSection, "PUT", "/site/captcha",
			`{"driver":"string","width":220,"height":64}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":0`) {
			t.Fatalf("先保存一份合法配置应成功，状态 %d，body: %s", rec.Code, rec.Body.String())
		}
		if rec := call(h.UpdateCaptchaSection, "PUT", "/site/captcha", `{"driver":"string","width":5000}`); !strings.Contains(rec.Body.String(), `"code":1001`) {
			t.Fatalf("越界配置应被拒绝，实际: %s", rec.Body.String())
		}

		read := call(h.AdminCaptchaSection, "GET", "/site/admin/captcha", "")
		if !strings.Contains(read.Body.String(), `"width":220`) || strings.Contains(read.Body.String(), `"width":5000`) {
			t.Fatalf("失败的那次保存不该改动配置，管理读实际: %s", read.Body.String())
		}
	})
}

// mentionsCaptchaField 判断失败文案是否点明了具体字段（而不是只给一句笼统的"不合法"）。
//
// 这两个用例失败过：协议类别一开始落在默认兜底（1002），且文案只有消息表里的通用句子，
// 字段级原因被盖掉 —— 运营看到"配置不合法"却不知道该改哪一项。
func mentionsCaptchaField(body string) bool {
	for _, field := range []string{"宽度", "类型", "背景色", "驱动", "字符数", "有效期", "字符集", "噪点", "倾斜"} {
		if strings.Contains(body, field) {
			return true
		}
	}
	return false
}
