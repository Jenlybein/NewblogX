package markdown

import (
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type ContentPart struct {
	Order   int      `json:"order,omitempty"`
	Level   int      `json:"level,omitempty"`
	Title   string   `json:"title,omitempty"`
	Path    []string `json:"path,omitempty"`
	Content string   `json:"content,omitempty"`
}

type partHeading struct {
	Level int
	Title string
	Start int
}

func MdToContentParts(md string) []ContentPart {
	md = normalizeMarkdownSyntax(md)
	source := []byte(md)
	doc := rawHTMLMarkdownEngine.Parser().Parse(text.NewReader(source))
	headings := collectPartHeadings(doc, source)
	if len(headings) == 0 {
		content := MdToPlainText(md)
		if content == "" {
			return nil
		}
		return []ContentPart{{
			Order:   0,
			Level:   0,
			Content: content,
		}}
	}

	parts := make([]ContentPart, 0, len(headings)+1)
	order := 0

	preface := MdToPlainText(string(source[:headings[0].Start]))
	if preface != "" {
		parts = append(parts, ContentPart{
			Order:   order,
			Level:   0,
			Content: preface,
		})
		order++
	}

	pathStack := make([]string, 0, 6)
	for index, heading := range headings {
		end := len(source)
		if index+1 < len(headings) {
			end = headings[index+1].Start
		}

		content := MdToPlainText(string(source[heading.Start:end]))
		if content == "" {
			continue
		}

		if heading.Level <= len(pathStack) {
			pathStack = append(pathStack[:heading.Level-1], heading.Title)
		} else {
			pathStack = append(pathStack, heading.Title)
		}

		parts = append(parts, ContentPart{
			Order:   order,
			Level:   heading.Level,
			Title:   heading.Title,
			Path:    append([]string(nil), pathStack...),
			Content: content,
		})
		order++
	}

	return parts
}

func collectPartHeadings(doc gast.Node, source []byte) []partHeading {
	headings := make([]partHeading, 0)
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*gast.Heading)
		if !ok || heading.Lines().Len() == 0 {
			continue
		}

		start := heading.Lines().At(0).Start
		title := MdToPlainText(string(heading.Lines().Value(source)))
		if title == "" {
			continue
		}

		headings = append(headings, partHeading{
			Level: heading.Level,
			Title: title,
			Start: start,
		})
	}
	return headings
}
