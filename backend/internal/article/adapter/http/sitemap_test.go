package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// newSitemapTestHandler 装一个只用内存 fake 的 article 处理器。
//
// 不碰数据库：站点地图是纯读、无副作用，fake 仓储足以覆盖"哪些文章该出、
// 绝对地址怎么拼"这两件真正会错的事。
func newSitemapTestHandler(t *testing.T) (*Handler, *apptest.Repos) {
	t.Helper()

	repos := apptest.NewRepos()
	// 投稿配额端口（票 304）与站点地图无关，但 `app.New` 的装配位置要求它显式给一个：
	// 传 nil 等于"这条路径一旦被牵到配额就 panic"，宁可用一个永远有额度的 fake。
	svc := app.New(apptest.NewTx(repos), apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), apptest.NewSubmitQuota(15))
	return NewHandler(svc, nil, nil, nil, nil), repos
}

// TestSitemapHandlerEmitsAbsoluteURLs 钉住站点地图的文件形态（#314 交付 1）。
//
//   - 必须带 `Content-Type: application/xml`：浏览器直接打开 / 爬虫抓取时按 XML
//     解析，返回 text/plain 或 JSON 都会让部分引擎直接判为无效；
//   - `<loc>` 必须是**绝对地址**（sitemap 协议要求），域名取请求的 Host ——
//     本地 / 测试 / 生产三种部署形态共用同一份代码；
//   - 首页要出现，私密页面（创作区、历史、消息、搜索）都不能出现；
//   - 只有已发布且可见的文章进清单。
func TestSitemapHandlerEmitsAbsoluteURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, repos := newSitemapTestHandler(t)
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Status(domain.StatusPublished).Build(),
		apptest.NewArticle().ID(ctype.NewID(12)).Status(domain.StatusDraft).Build(),
	)

	body, header := sitemapRequest(t, h, "http://blog.example.com", nil)

	if !strings.HasPrefix(header.Get("Content-Type"), "application/xml") {
		t.Fatalf("Content-Type 应为 application/xml，实际 %q", header.Get("Content-Type"))
	}
	for _, want := range []string{
		`<loc>http://blog.example.com/</loc>`,
		`<loc>http://blog.example.com/article/11</loc>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("站点地图缺少 %s，实际输出：\n%s", want, body)
		}
	}
	if strings.Contains(body, "/article/12") {
		t.Fatalf("草稿不能出现在站点地图里：\n%s", body)
	}
	// 客户端页面（合集 / 项目 / 个人主页 / 搜索）不在渲染边界内，爬虫只会拿到
	// 空壳——列进站点地图等于让站长后台长期显示「已提交，未收录」。
	for _, privatePath := range []string{"/search", "/history", "/studio", "/messages", "/collections", "/projects", "/users"} {
		if strings.Contains(body, "<loc>http://blog.example.com"+privatePath) {
			t.Fatalf("私密/客户端页面不能进站点地图: %s\n%s", privatePath, body)
		}
	}
}

// TestSitemapHandlerHonorsForwardedProto 钉住反向代理场景的协议还原。
//
// 生产里 nginx 终止 TLS，后端只看到 http；不看 X-Forwarded-Proto 会输出
// `http://` 的绝对地址，而站点只监听 https —— 那批 URL 在爬虫眼里是失效的。
func TestSitemapHandlerHonorsForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, repos := newSitemapTestHandler(t)
	repos.FakeArticles().Seed(apptest.NewArticle().ID(ctype.NewID(21)).Status(domain.StatusPublished).Build())

	body, _ := sitemapRequest(t, h, "http://blogx.gentlybeing.cn", map[string]string{
		"X-Forwarded-Proto": "https",
	})

	if !strings.Contains(body, "<loc>https://blogx.gentlybeing.cn/article/21</loc>") {
		t.Fatalf("应使用 X-Forwarded-Proto 还原 https 绝对地址，实际输出：\n%s", body)
	}
}

// TestSitemapHandlerMarksLastmod 钉住 <lastmod> 的格式（W3C datetime，UTC）。
//
// 缺少 lastmod 的站点地图仍合法，但爬虫只能靠启发式判断"要不要重抓"；
// 格式写错（本地时区、空格分隔）则整条 URL 被忽略。
func TestSitemapHandlerMarksLastmod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, repos := newSitemapTestHandler(t)
	repos.FakeArticles().Seed(apptest.NewArticle().ID(ctype.NewID(31)).
		Status(domain.StatusPublished).CreatedAt(apptest.DefaultTime).Build())

	body, _ := sitemapRequest(t, h, "http://blog.example.com", nil)

	if !strings.Contains(body, "<lastmod>2026-01-01T12:00:00Z</lastmod>") {
		t.Fatalf("lastmod 应为 UTC 的 W3C 时间，实际输出：\n%s", body)
	}
}

// TestSitemapIsMountedAtAPIPrefix 钉住新端点的挂载路径（#314 交付 1）。
//
// 上面几个用例都把 handler 直接挂在测试引擎上，只能证明"这个函数能产出 XML"。
// 这条走一遍真实的 Register：路由写错位置（例如挂进 /articles 下，
// 会被 `/articles/:id` 抢先吃掉 "sitemap.xml"）或拼错前缀都会在这里红。
func TestSitemapIsMountedAtAPIPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, repos := newSitemapTestHandler(t)
	repos.FakeArticles().Seed(apptest.NewArticle().ID(ctype.NewID(41)).Status(domain.StatusPublished).Build())

	engine := gin.New()
	noop := func(c *gin.Context) { c.Next() }
	h.Register(engine.Group("/api"), noop, noop)

	req := httptest.NewRequest(http.MethodGet, "/api/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sitemap.xml 应返回 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<loc>http://example.com/article/41</loc>") {
		t.Fatalf("站点地图内容不对：\n%s", rec.Body.String())
	}
}

// sitemapRequest 发一次站点地图请求，返回响应体与响应头。
func sitemapRequest(t *testing.T, h *Handler, origin string, headers map[string]string) (string, http.Header) {
	t.Helper()

	engine := gin.New()
	engine.GET("/api/sitemap.xml", h.Sitemap)

	req := httptest.NewRequest(http.MethodGet, origin+"/api/sitemap.xml", nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("站点地图应返回 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String(), rec.Header()
}
