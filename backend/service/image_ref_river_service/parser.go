package image_ref_river_service

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var htmlImageSrcRegexp = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)

// ParseMarkdownImageURLs 解析 Markdown 内的图片 URL
func ParseMarkdownImageURLs(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	lowerContent := strings.ToLower(content)
	if !strings.Contains(content, "![") && !strings.Contains(lowerContent, "<img") {
		return nil
	}

	source := []byte(content)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))

	result := make([]string, 0, 8)
	appendURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		result = append(result, raw)
	}

	_ = gast.Walk(doc, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		imageNode, ok := node.(*gast.Image)
		if ok {
			appendURL(string(imageNode.Destination))
		}
		return gast.WalkContinue, nil
	})

	for _, matches := range htmlImageSrcRegexp.FindAllStringSubmatch(content, -1) {
		if len(matches) > 1 {
			appendURL(matches[1])
		}
	}
	return result
}
