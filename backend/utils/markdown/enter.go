package markdown

import (
	"bytes"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark/text"
)

// 不允许任何 HTML 标签存在，纯文本输出
func MdToText(md string) string {
	raw := getRawHTML(md)
	// 移除所有 HTML 标签，只保留文本内容
	return bluemonday.StrictPolicy().Sanitize(raw)
}

// 纯文本输出，且清空所有换行和冗余空白，适合单个段落使用
func MdToTextParagraph(md string) string {
	text := MdToText(md)
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

// 纯文本输出，且清空所有冗余空白
func MdToPlainText(value string) string {
	value = MdToText(value)

	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")

	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// 允许所有 HTML 标签存在，不做任何过滤
func MdToHTMLUnsafe(md string) string {
	return getRawHTML(md)
}

// 过滤掉不安全的 HTML，如 <script>, <onerror> 等
func MdToHTMLSafe(md string) string {
	raw := getRawHTML(md)

	// 以 UGC 策略为基础（允许基本格式、链接等）
	// UGC策略已经很严格，想再禁用一些标签则使用SkipElementsContent
	p := bluemonday.UGCPolicy()

	// 允许数学公式用到的 class 和标签
	p.AllowAttrs("class").Matching(reMath).OnElements("span", "div")

	// 放行标题 id（用于标题锚点）
	p.AllowAttrs("id").Matching(reUserContentID).OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	// 放行图片缩放（Typora 常用）
	p.AllowStyles("zoom").Matching(reZoomValue).OnElements("img")

	return p.Sanitize(raw)
}

// Markdown 内容过滤，确保安全合规
func MdToSafe(md string) string {
	md = normalizeMarkdownSyntax(md)
	source := []byte(md)
	doc := safeMarkdownEngine.Parser().Parse(text.NewReader(source))
	sanitizeSafeMarkdownAST(doc, source)

	var buf bytes.Buffer
	if err := safeMarkdownEngine.Renderer().Render(&buf, source, doc); err != nil {
		return ""
	}
	return buf.String()
}

// 提取纯文本前 n 个字符
func ExtractText(text string, length int) string {
	if length <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) > length {
		return string(runes[:length])
	}
	return text
}
