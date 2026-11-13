package http

import (
	"encoding/xml"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
)

// Sitemap 输出站点地图（sitemap.xml，#314 交付 1）。
//
// 为什么是一个接口而不是前端静态文件：能进这份清单的文章由**后端**的可见性规则
// 决定（已发布 + visible + 「显示非管理员发布文章」开关），前端拿不到这份判定，
// 也不该再复制一份。前端只负责告诉爬虫"站点地图在哪"（robots.txt 的 Sitemap: 行）。
//
// 绝对地址的域名取请求的 Host（生产里 nginx 传 `Host $host`），协议取
// `X-Forwarded-Proto`：后端只监听 http，生产由 nginx 终止 TLS，不看这个头
// 就会输出 `http://` 的地址，而那批地址在只监听 https 的站点上是失效的。
//
// 刻意**只含首页与文章页**：合集 / 项目 / 个人主页 / 搜索页都是 `ssr: false`
// 的客户端页面（渲染边界 ADR-0016 决策 1），爬虫打开只拿到空壳。把它们列进来
// 不会带来收录，只会让站长后台长期显示「已提交，未收录」。要收录它们得先扩
// SSR 白名单，那是另一张票。
//
// @Summary 站点地图
// @Description 只含首页与已发布且可见的文章页（sitemap 协议）
// @Tags article
// @Produce application/xml
// @Success 200 {string} string "sitemap.xml"
// @Router /sitemap.xml [get]
func (h *Handler) Sitemap(c *gin.Context) {
	entries, err := h.svc.Sitemap(c.Request.Context(), h.onlyAdminAuthors())
	if err != nil {
		h.fail(c, err)
		return
	}

	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(renderSitemap(siteOrigin(c), entries)))
}

// siteOrigin 还原"读者看到的站点根地址"（scheme + host），不含结尾斜杠。
func siteOrigin(c *gin.Context) string {
	scheme := "http"
	if proto := strings.TrimSpace(strings.ToLower(c.GetHeader("X-Forwarded-Proto"))); proto == "http" || proto == "https" {
		scheme = proto
	} else if c.Request.TLS != nil {
		scheme = "https"
	}

	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		// Host 缺失时退化成相对路径也能被部分引擎接受，但 sitemap 协议要求绝对
		// 地址，所以这里宁可给一个合法但不指向任何机器的占位，也不输出 "://"。
		host = "localhost"
	}
	return scheme + "://" + host
}

// renderSitemap 把首页与文章条目渲染成 sitemap.xml。
//
// 首页固定排第一条、不带 lastmod（站点首页没有单一的"最后修改时间"，
// 编造一个只会让爬虫频繁回抓整站）。
func renderSitemap(origin string, entries []app.SitemapEntry) string {
	var b strings.Builder

	b.WriteString(xml.Header)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	writeSitemapURL(&b, origin+"/", "")

	for _, e := range entries {
		lastmod := ""
		if !e.Lastmod.IsZero() {
			lastmod = e.Lastmod.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		writeSitemapURL(&b, origin+e.Path, lastmod)
	}

	b.WriteString("</urlset>\n")
	return b.String()
}

// writeSitemapURL 追加一条 <url> 块。lastmod 为空时不输出该字段。
func writeSitemapURL(b *strings.Builder, loc, lastmod string) {
	b.WriteString("  <url>\n    <loc>")
	b.WriteString(escapeXML(loc))
	b.WriteString("</loc>\n")
	if lastmod != "" {
		b.WriteString("    <lastmod>")
		b.WriteString(escapeXML(lastmod))
		b.WriteString("</lastmod>\n")
	}
	b.WriteString("  </url>\n")
}

// escapeXML 转义 XML 文本节点。
//
// 现在的 loc 全是数字 ID，理论上不会出现特殊字符；仍然转义是因为转义只有一次
// 函数调用的成本，而漏转义时的失败形态是"整份 sitemap 被引擎判为无效"。
func escapeXML(value string) string {
	var buf strings.Builder
	if err := xml.EscapeText(&buf, []byte(value)); err != nil {
		return value
	}
	return buf.String()
}
