package markdown_test

import (
	"myblogx/utils/markdown"
	"strings"
	"testing"
)

func TestMarkdownHelpers(t *testing.T) {
	md := "# 标题\n\n<script>alert(1)</script>\n\n[跳转](#标题)"
	unsafe := markdown.MdToHTMLUnsafe(md)
	if !strings.Contains(unsafe, "<script>") {
		t.Fatal("MdToHTMLUnsafe 不应过滤 script")
	}
	if !strings.Contains(unsafe, `href="#`) {
		t.Fatal("Markdown 链接应被渲染为 HTML 链接")
	}

	safe := markdown.MdToHTMLSafe(md)
	if strings.Contains(safe, "<script>") {
		t.Fatal("MdToHTMLSafe 应过滤 script")
	}
	if !strings.Contains(safe, `href="#`) {
		t.Fatal("安全模式应保留普通锚点链接")
	}

	text := markdown.MdToText("**hello**")
	if strings.Contains(text, "<") {
		t.Fatalf("MdToText 应返回纯文本: %s", text)
	}
	if got := markdown.MdToText("[这是一个链接](### 这是一个 标题 )"); strings.Contains(got, "[") || strings.Contains(got, "](") || !strings.Contains(got, "这是一个链接") {
		t.Fatalf("MdToText 应先纠正错误链接语法再提取纯文本: %s", got)
	}

	if got := markdown.ExtractText("abcdef", 3); got != "abc" {
		t.Fatalf("ExtractText 截断错误: %s", got)
	}
	if got := markdown.ExtractText("你好世界", 3); got != "你好世" {
		t.Fatalf("ExtractText 应按 rune 截断中文: %s", got)
	}
	if got := markdown.ExtractText("你好", 10); got != "你好" {
		t.Fatalf("ExtractText 不应在 rune 长度不足时越界: %s", got)
	}
	if got := markdown.ExtractText("hello", 0); got != "" {
		t.Fatalf("ExtractText 在长度<=0时应返回空串: %q", got)
	}
}

func TestMdToSafe(t *testing.T) {
	md := "# 标题\n\n<script>alert(1)</script>\n\n[跳转](#标题)\n\n[危险](javascript:alert(1))\n\n![图](javascript:alert(2))"
	safe := markdown.MdToSafe(md)

	if strings.Contains(safe, "<script>") {
		t.Fatalf("MdToSafe 应移除原生 HTML: %s", safe)
	}
	if strings.Contains(safe, "javascript:") {
		t.Fatalf("MdToSafe 应移除危险链接: %s", safe)
	}
	if !strings.Contains(safe, "[跳转](#标题)") {
		t.Fatalf("MdToSafe 应保留原始锚点链接: %s", safe)
	}
}

func TestMdToSafeKeepsAllowedHTML(t *testing.T) {
	md := `<span class="math inline">x^2</span>

<img src="https://example.com/a.png" style="zoom:50%">`

	safe := markdown.MdToSafe(md)
	if !strings.Contains(safe, `<span>x^2</span>`) {
		t.Fatalf("MdToSafe 应保留安全 HTML 内容: %s", safe)
	}
	if strings.Contains(safe, `class="math inline"`) {
		t.Fatalf("MdToSafe 不应保留前端渲染不需要的 math class: %s", safe)
	}
	if !strings.Contains(safe, `style="zoom: 50%"`) {
		t.Fatalf("MdToSafe 应保留图片 zoom 样式: %s", safe)
	}
}

func TestMdToHTMLSafeKeepsMathClass(t *testing.T) {
	md := `<span class="math inline">x^2</span>`
	safe := markdown.MdToHTMLSafe(md)

	if !strings.Contains(safe, `<span class="math inline">x^2</span>`) {
		t.Fatalf("MdToHTMLSafe 应保留 math class: %s", safe)
	}
}

// TestMdToSafeKeepsDrawingSVG 是「正文放行内联 SVG」的第一层（#135）。
//
// 入库消毒此前把 <svg> **整块删除**（不是转义），于是图压根没进数据库，
// 后续任何渲染层都拿不到它。这里钉住：绘图元素与绘图属性要完整保留。
func TestMdToSafeKeepsDrawingSVG(t *testing.T) {
	md := `<div class="svg-block"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 820 502" role="img" style="width:100%;max-width:760px;height:auto">
<title>Agent Loop 循环</title><desc>用户输入经 LLM 推理</desc>
<g><rect x="0" y="0" width="820" height="502" rx="16" fill="#fbfcfd"/><circle cx="50" cy="50" r="40" fill="red"/><path d="M330,124 L330,138" stroke="#3f6ea6" stroke-width="2"/><text x="330" y="87" font-size="15.5" fill="#1f2430">用户输入</text><polygon points="330,224 456,268 330,312"/><ellipse cx="60" cy="60" rx="20" ry="10"/><line x1="0" y1="0" x2="10" y2="10"/><polyline points="1,1 2,2"/><tspan x="1" y="2">t</tspan></g>
</svg></div>`

	safe := markdown.MdToSafe(md)

	// 绘图元素必须保留（被删除或转义都算失败）。
	if !strings.Contains(safe, "<svg") {
		t.Fatalf("MdToSafe 应保留 svg 元素: %s", safe)
	}
	for _, element := range []string{"<rect", "<circle", "<path", "<text", "<polygon", "<ellipse", "<line", "<polyline", "<tspan", "<g>", "<title", "<desc"} {
		if !strings.Contains(safe, element) {
			t.Fatalf("MdToSafe 应保留绘图元素 %s: %s", element, safe)
		}
	}
	// 绘图属性必须保留。
	//
	// 用大小写不敏感的比较：HTML tokenizer 会把属性名小写化
	// （viewBox → viewbox），而浏览器在 HTML 内联 SVG 时会按 HTML 规范的
	// "SVG attribute adjustment" 把它映射回 viewBox，渲染不受影响。
	// 所以这里钉住的是"属性语义还在"，不是"属性名的原始大小写"。
	lowered := strings.ToLower(safe)
	for _, attr := range []string{"viewbox", "d=", "fill=", "stroke=", "stroke-width", "points="} {
		if !strings.Contains(lowered, strings.ToLower(attr)) {
			t.Fatalf("MdToSafe 应保留绘图属性 %s: %s", attr, safe)
		}
	}
	// 排版包裹与 class 必须保留（"图 + 图注"写法依赖它）。
	if !strings.Contains(safe, "<div") || !strings.Contains(safe, `class="svg-block"`) {
		t.Fatalf("MdToSafe 应保留排版包裹与 class: %s", safe)
	}
}

// TestMdToSafeStripsDangerousSVG 钉住放行 SVG 之后危险面仍然封闭。
//
// SVG 是可执行脚本的载体：可内嵌 script、onload 事件、javascript: 链接，
// 以及能装任意 HTML 的 foreignObject。这些一个都不能放进来。
func TestMdToSafeStripsDangerousSVG(t *testing.T) {
	md := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" onload="alert(1)">
<script>alert(2)</script>
<foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><iframe src="javascript:alert(3)"></iframe></body></foreignObject>
<rect x="0" y="0" width="10" height="10" fill="red" onclick="alert(4)"/>
<animate attributeName="x" from="0" to="10" dur="1s"/>
</svg>`

	safe := markdown.MdToSafe(md)

	for _, dangerous := range []string{"<script", "onload", "onclick", "javascript:", "<foreignObject", "<foreignObject ", "<animate", "<iframe"} {
		if strings.Contains(safe, dangerous) {
			t.Fatalf("MdToSafe 不应放行危险内容 %q: %s", dangerous, safe)
		}
	}
	// 危险内容被剥掉后，安全的绘图元素仍应在。
	if !strings.Contains(safe, "<rect") {
		t.Fatalf("MdToSafe 应保留同一片段里的安全绘图元素: %s", safe)
	}
}

// TestMdToSafeKeepsLayoutTagsForFigureCaption 钉住"图 + 图注"的排版写法。
func TestMdToSafeKeepsLayoutTagsForFigureCaption(t *testing.T) {
	md := "<div class=\"svg-block\">\n\n<svg viewBox=\"0 0 10 10\"><rect width=\"10\" height=\"10\"/></svg>\n\n<center>图 1　Agent Loop：推理 → 判定 → 执行 → 回注</center>\n\n</div>"

	safe := markdown.MdToSafe(md)

	if !strings.Contains(safe, "<center") {
		t.Fatalf("MdToSafe 应保留 center 图注标签: %s", safe)
	}
	if !strings.Contains(safe, "图 1") {
		t.Fatalf("MdToSafe 不应丢掉图注文字: %s", safe)
	}
	if !strings.Contains(safe, `class="svg-block"`) {
		t.Fatalf("MdToSafe 应保留 svg-block 包裹的 class: %s", safe)
	}
}

func TestMdToSafeKeepsMathSyntax(t *testing.T) {
	md := "行内公式 $a+b$\n\n$$\nx^2+y^2=z^2\n$$"
	safe := markdown.MdToSafe(md)

	if !strings.Contains(safe, "$a+b$") {
		t.Fatalf("MdToSafe 应保留行内数学公式: %s", safe)
	}
	if !strings.Contains(safe, "$$") || !strings.Contains(safe, "x^2+y^2=z^2") {
		t.Fatalf("MdToSafe 应保留块级数学公式: %s", safe)
	}
}

func TestMdToSafeDropsMalformedURL(t *testing.T) {
	md := "[异常链接](http://[::1)"
	safe := markdown.MdToSafe(md)

	if strings.Contains(safe, "http://[::1") {
		t.Fatalf("MdToSafe 不应保留解析失败的链接: %s", safe)
	}
}

func TestMdToSafeNormalizeMalformedHeadingLink(t *testing.T) {
	md := "[这是一个链接](### 这是一个 标题 )"
	safe := markdown.MdToSafe(md)

	if strings.Contains(safe, "](### ") {
		t.Fatalf("MdToSafe 应纠正误写成标题语法的链接目标: %s", safe)
	}
	if !strings.Contains(safe, "](#这是一个-标题)") {
		t.Fatalf("MdToSafe 应转换为合法锚点链接: %s", safe)
	}
}

func TestMdToContentParts(t *testing.T) {
	md := "# 一级标题\n你好啊啊啊\n## 二级标题 1\n哈哈哈哈\n### 三级标题 1\n## 二级标题 2\n测试测试"
	parts := markdown.MdToContentParts(md)

	if len(parts) != 4 {
		t.Fatalf("content_parts 数量错误: %+v", parts)
	}
	if parts[0].Level != 1 || parts[0].Title != "一级标题" || parts[0].Content != "一级标题\n你好啊啊啊" {
		t.Fatalf("一级标题分段错误: %+v", parts[0])
	}
	if len(parts[1].Path) != 2 || parts[1].Path[0] != "一级标题" || parts[1].Path[1] != "二级标题 1" {
		t.Fatalf("二级标题路径错误: %+v", parts[1])
	}
	if parts[2].Level != 3 || parts[2].Content != "三级标题 1" {
		t.Fatalf("三级标题分段错误: %+v", parts[2])
	}
	if parts[3].Content != "二级标题 2\n测试测试" {
		t.Fatalf("末尾分段错误: %+v", parts[3])
	}
	if got := markdown.ExtractText(markdown.MdToTextParagraph(md), 6); got != "一级标题 你" {
		t.Fatalf("content_head 生成错误: %q", got)
	}
}

func TestMdToContentPartsStripMarkdownFormat(t *testing.T) {
	md := "# 一级 **标题**\n这是 [链接](https://example.com) 和 `代码`\n- 列表1\n- 列表2"
	parts := markdown.MdToContentParts(md)

	if len(parts) != 1 {
		t.Fatalf("content_parts 数量错误: %+v", parts)
	}

	content := parts[0].Content
	markdownTokens := []string{"**", "[", "](", "`", "# "}
	for _, token := range markdownTokens {
		if strings.Contains(content, token) {
			t.Fatalf("content_parts 不应保留 Markdown 格式 token=%q content=%q", token, content)
		}
	}

	if !strings.Contains(content, "链接") || !strings.Contains(content, "代码") || !strings.Contains(content, "列表1") {
		t.Fatalf("content_parts 纯文本内容错误: %q", content)
	}
}

func TestMdToContentPartsNormalizeMalformedHeadingLink(t *testing.T) {
	md := "# 这是一个 标题\n\n[这是一个链接](### 这是一个 标题 )"
	parts := markdown.MdToContentParts(md)

	if len(parts) != 1 {
		t.Fatalf("content_parts 数量错误: %+v", parts)
	}
	if strings.Contains(parts[0].Content, "[") || strings.Contains(parts[0].Content, "](") {
		t.Fatalf("content_parts 不应保留残余 Markdown 链接语法: %q", parts[0].Content)
	}
	if !strings.Contains(parts[0].Content, "这是一个链接") {
		t.Fatalf("content_parts 应保留链接文本: %q", parts[0].Content)
	}
	if got := markdown.ExtractText(markdown.MdToTextParagraph(md), 100); strings.Contains(got, "[") || strings.Contains(got, "](") {
		t.Fatalf("content_head 不应保留残余 Markdown 链接语法: %q", got)
	}
}
