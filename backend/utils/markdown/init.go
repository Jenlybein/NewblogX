package markdown

import (
	"bytes"
	"regexp"
	"strings"

	mdrenderer "github.com/blackstork-io/goldmark-markdown"
	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var (
	// 图片缩放正则表达式
	reZoomValue = regexp.MustCompile(`^((0\.\d+|1(\.\d+)?|2(\.0+)?)|([1-9]\d?%|1\d{2}%|200%))$`)
	// 匹配 math, math inline, math display
	reMath = regexp.MustCompile(`^math(\s.+)?$`)
	// 匹配 user-content- 开头的 id
	reUserContentID = regexp.MustCompile(`^user-content-.*$`)
	// 匹配把标题语法误写到链接目标里的历史内容，例如 [文本](### 标题)
	reMalformedHeadingLink = regexp.MustCompile(`\[((?:\\.|[^\]])+)\]\((#{1,6})\s+([^)]+?)\s*\)`)
)

// newRawHTMLEngine 造一份"原样输出 HTML"的引擎。
func newRawHTMLEngine() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			mathjax.MathJax,
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)
}

// newSafeEngine 造一份"Markdown→Markdown 消毒"的引擎。
func newSafeEngine() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			mathjax.MathJax,
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRenderer(mdrenderer.NewRenderer()),
	)
}

// rawHTMLMarkdownEngine 可以全局共用一份：
// goldmark 自带的 html 渲染器每次 Render 都会新建渲染上下文，实例本身不留存状态。
// 这一点已由 md_concurrency_test.go 的并发用例 + -race 验证。
var rawHTMLMarkdownEngine = newRawHTMLEngine()

// 而挂了 mdrenderer 的这一份**必须每次调用新建，绝不能复用**（M4 收尾项 ①）。
//
// 根因：mdrenderer.Renderer 把渲染中间状态（previousLineBlank / wasAtNL / bufs 等）
// 存放在渲染器**实例**上。构造时 previousLineBlank 被初始化为 true
// （源码注释就是 "document (conceptually) starts with a blank line"），
// 但 Render 结束时不会把它复位、Render 开头也不会重置——
// 于是**复用实例必然继承上一次渲染留下的脏状态**。
//
// 症状：
//   轻则同一份输入在不同调用次序下产出不同结果（偶发多出/少掉前导换行）；
//   并发共用时更严重——flush 阶段会 index out of range [-1] panic，直接打挂请求。
//
// 为什么不能用 sync.Pool 复用：残留状态是粘在实例上的，
// "归还再取出"只是换一个 goroutine 来继承脏状态，并不能清零；
// 而这些字段在 internal 包里是私有的，外部也无法手动重置。
// 所以唯一正确的做法是每次新建一份干净引擎。
//
// 代价可接受：MdToSafe 只在文章创建/更新两处写路径调用，不在读路径上。
// 复现与回归见 md_concurrency_test.go。
//
// 下方不再保留 safeEngine 的包级变量，就是为了杜绝有人不小心又把它共享出去。

func getRawHTML(md string) string {
	md = normalizeMarkdownSyntax(md)
	source := []byte(md)
	doc := rawHTMLMarkdownEngine.Parser().Parse(text.NewReader(source))

	var buf bytes.Buffer
	if err := rawHTMLMarkdownEngine.Renderer().Render(&buf, source, doc); err != nil {
		return ""
	}
	return buf.String()
}

func normalizeMarkdownSyntax(md string) string {
	if md == "" {
		return ""
	}

	return reMalformedHeadingLink.ReplaceAllStringFunc(md, func(match string) string {
		submatches := reMalformedHeadingLink.FindStringSubmatch(match)
		if len(submatches) != 4 {
			return match
		}

		linkText := submatches[1]
		title := strings.TrimSpace(submatches[3])
		if title == "" {
			return match
		}

		title = strings.Join(strings.Fields(title), "-")

		return "[" + linkText + "](#" + title + ")"
	})
}
