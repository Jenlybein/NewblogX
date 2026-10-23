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
