package markdown

import (
	"bytes"
	"net/url"
	"strings"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/microcosm-cc/bluemonday"
	gast "github.com/yuin/goldmark/ast"
)

// 过滤掉不安全的 HTML
func sanitizeSafeHTMLFragment(raw string) string {
	// 以 UGC 策略为基础（允许基本格式、链接等）
	p := bluemonday.UGCPolicy()
	// 允许图片缩放标签
	p.AllowStyles("zoom").Matching(reZoomValue).OnElements("img")

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
