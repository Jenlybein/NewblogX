package markdown

import (
	"bytes"
	"net/url"
	"strings"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/microcosm-cc/bluemonday"
	gast "github.com/yuin/goldmark/ast"
)

// svgDrawingElements 是放行的 SVG 绘图元素（#135 / ADR-0021）。
//
// 只到"画图"为止：能表达流程图、架构图、示意图。刻意**不含**
// script / foreignObject / animate* —— 它们是可执行或可装载任意 HTML 的面。
var svgDrawingElements = []string{
	"svg", "g", "title", "desc",
	"rect", "circle", "ellipse", "line", "polyline", "polygon", "path",
	"text", "tspan",
}

// svgDrawingAttrs 是放行的绘图属性。
//
// 注意属性名一律小写：HTML tokenizer 会把属性名小写化，bluemonday 也按小写匹配。
// 浏览器在 HTML 内联 SVG 时会把 viewbox 正确映射回 viewBox，因此不影响渲染。
var svgDrawingAttrs = []string{
	// 画布与定位
	"xmlns", "viewbox", "preserveaspectratio", "role", "aria-label", "style",
	"x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry",
	"width", "height",
	// 路径与多边形
	"d", "points",
	// 描边与填色
	"fill", "fill-opacity", "stroke", "stroke-width", "stroke-opacity",
	"stroke-linecap", "stroke-dasharray", "opacity",
	// 文字
	"font-family", "font-size", "font-weight", "text-anchor", "letter-spacing",
	"dominant-baseline", "transform",
	// 图片标识（供引用与无障碍使用）
	"id", "class",
}

// layoutElements 是放行的排版标签，用于"图 + 图注"的包裹写法。
//
// center / figure / figcaption 让图注能居中与语义化；div 承载 svg-block 之类的包裹。
var layoutElements = []string{"div", "center", "figure", "figcaption"}

// 过滤掉不安全的 HTML
func sanitizeSafeHTMLFragment(raw string) string {
	// 以 UGC 策略为基础（允许基本格式、链接等）
	p := bluemonday.UGCPolicy()
	// 允许图片缩放标签
	p.AllowStyles("zoom").Matching(reZoomValue).OnElements("img")

	// 放行 SVG 绘图元素与属性。UGCPolicy 本身不含 svg，这也是此前
	// <svg> 在入库时被整块删除的原因（#135 / ADR-0021）。
	p.AllowElements(svgDrawingElements...)
	p.AllowAttrs(svgDrawingAttrs...).OnElements(svgDrawingElements...)

	// 关键：允许"不带属性"的绘图元素。
	//
	// bluemonday 对**没有属性**的起始标签会额外查一次 allowNoAttrs，
	// 不在该集合里就直接丢掉标签（只留内容）。SVG 里 <g>/<desc>/<text>
	// 这类容器元素经常不带属性，少了这一步它们会被剥成纯文本——
	// 现象是图散架、只剩文字。
	p.AllowNoAttrs().OnElements(svgDrawingElements...)

	// 放行"图 + 图注"的排版包裹，并保留 class（svg-block 之类的包裹标识）。
	p.AllowElements(layoutElements...)
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements(layoutElements...)
	// svg 自身的 class 也保留（已在 svgDrawingAttrs 里，这里显式收紧取值形态）。
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("svg")

	return p.Sanitize(raw)
}

// 统一的 AST 变更操作记录
type astMutation struct {
	parent   gast.Node
	oldNode  gast.Node
	newNodes []gast.Node
}

func sanitizeSafeMarkdownAST(doc gast.Node, source []byte) {
	var mutations []astMutation

	_ = gast.Walk(doc, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}

		switch item := node.(type) {
		case *gast.Link:
			item.Destination = []byte(filterSafeDestination(string(item.Destination), false))
		case *gast.Image:
			item.Destination = []byte(filterSafeDestination(string(item.Destination), true))

		case *mathjax.InlineMath, *mathjax.MathBlock, *gast.HTMLBlock, *gast.RawHTML:
			if node.Parent() == nil {
				break
			}

			var newNodes []gast.Node
			switch n := item.(type) {
			case *mathjax.InlineMath:
				newNodes = buildSafeInlineMathNodes(n, source)
			case *mathjax.MathBlock:
				newNodes = buildSafeMathBlockNodes(n, source)
			case *gast.HTMLBlock:
				newNodes = sanitizeSafeHTMLBlockNode(n, source)
			case *gast.RawHTML:
				newNodes = sanitizeSafeRawHTMLNode(n, source)
			}

			mutations = append(mutations, astMutation{
				parent:   node.Parent(),
				oldNode:  node,
				newNodes: newNodes,
			})
			return gast.WalkSkipChildren, nil
		}
		return gast.WalkContinue, nil
	})

	for _, m := range mutations {
		if m.parent == nil || m.oldNode == nil {
			continue
		}
		if len(m.newNodes) == 0 {
			m.parent.RemoveChild(m.parent, m.oldNode)
			continue
		}

		first := m.newNodes[0]
		m.parent.ReplaceChild(m.parent, m.oldNode, first)
		prev := first
		for _, next := range m.newNodes[1:] {
			m.parent.InsertAfter(m.parent, prev, next)
			prev = next
		}
	}
}

func sanitizeSafeHTMLBlockNode(node *gast.HTMLBlock, source []byte) []gast.Node {
	sanitized := strings.TrimSpace(sanitizeSafeHTMLFragment(string(node.Text(source))))
	if sanitized == "" {
		return nil
	}
	textBlock := gast.NewTextBlock()
	rawNode := gast.NewString([]byte(sanitized))
	rawNode.SetRaw(true)
	textBlock.AppendChild(textBlock, rawNode)
	return []gast.Node{textBlock}
}

func sanitizeSafeRawHTMLNode(node *gast.RawHTML, source []byte) []gast.Node {
	sanitized := strings.TrimSpace(sanitizeSafeHTMLFragment(string(node.Text(source))))
	if sanitized == "" {
		return nil
	}
	rawNode := gast.NewString([]byte(sanitized))
	rawNode.SetRaw(true)
	return []gast.Node{rawNode}
}

func buildSafeInlineMathNodes(node *mathjax.InlineMath, source []byte) []gast.Node {
	rawNode := gast.NewString([]byte("$" + extractInlineMathText(node, source) + "$"))
	rawNode.SetRaw(true)
	return []gast.Node{rawNode}
}

func buildSafeMathBlockNodes(node *mathjax.MathBlock, source []byte) []gast.Node {
	content := strings.TrimRight(extractMathBlockText(node, source), "\n")
	textBlock := gast.NewTextBlock()
	rawNode := gast.NewString([]byte("$$\n" + content + "\n$$"))
	rawNode.SetRaw(true)
	textBlock.AppendChild(textBlock, rawNode)
	return []gast.Node{textBlock}
}

func extractInlineMathText(node gast.Node, source []byte) string {
	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*gast.Text); ok {
			buf.Write(textNode.Segment.Value(source))
		}
	}
	return buf.String()
}

func extractMathBlockText(node *mathjax.MathBlock, source []byte) string {
	var buf bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

func filterSafeDestination(dest string, isImage bool) string {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return ""
	}
	if strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "./") || strings.HasPrefix(dest, "../") {
		return dest
	}

	u, err := url.Parse(dest)
	if err != nil {
		return ""
	}
	if u.Scheme == "" {
		return dest
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" || scheme == "https" {
		return dest
	}

	// 图片只允许 http/https
	if isImage {
		return ""
	}

	// 链接额外允许 mailto/tel
	if scheme == "mailto" || scheme == "tel" {
		return dest
	}

	return ""
}
