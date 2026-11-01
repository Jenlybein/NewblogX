package markdown_test

import (
	"strings"
	"testing"

	"myblogx/internal/platform/cdc/imageref"
	"myblogx/utils/markdown"
)

// svgFigureSource 是一个"图 + 图注 + 正文"的完整写作形态。
const svgFigureSource = "<div class=\"svg-block\">\n\n" +
	"<svg viewBox=\"0 0 10 10\"><title>图标题</title><desc>图的描述</desc><text>图里的文字</text><rect width=\"10\" height=\"10\"/></svg>\n\n" +
	"<center>图 1　说明文字</center>\n\n</div>\n\n这是正文段落。\n\n![图](https://cdn.example.com/a.png)\n"

// TestSvgTextDoesNotLeakIntoAbstract 钉住：SVG **图形内部的文字**不进摘要。
//
// 摘要与搜索投影的 content_head 都由 MdToText 派生。图形里的文字（<title>/
// <desc>/<text>）是图的内部标注，不属于正文叙述；把"图里的文字"塞进摘要
// 会让列表页出现看不懂的碎片。图注（<center>）不属于图形内部，算正文。
func TestSvgTextDoesNotLeakIntoAbstract(t *testing.T) {
	safe := markdown.MdToSafe(svgFigureSource)

	text := markdown.MdToText(safe)

	for _, leaked := range []string{"图里的文字", "图标题", "图的描述"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("SVG 图形内部文字 %q 不应进入摘要，实际摘要: %q", leaked, text)
		}
	}
	// 图注与正文必须保留 —— 它们是作者写的叙述。
	for _, kept := range []string{"图 1", "说明文字", "这是正文段落"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("摘要应保留正文/图注 %q，实际: %q", kept, text)
		}
	}
}

// TestSvgTextDoesNotLeakIntoContentHead 钉住同样的行为在单段落模式成立
// （ES 投影的 content_head 走 MdToTextParagraph）。
func TestSvgTextDoesNotLeakIntoContentHead(t *testing.T) {
	safe := markdown.MdToSafe(svgFigureSource)

	head := markdown.MdToTextParagraph(safe)

	if strings.Contains(head, "图里的文字") {
		t.Fatalf("content_head 不应含 SVG 内部文字，实际: %q", head)
	}
	if !strings.Contains(head, "这是正文段落") {
		t.Fatalf("content_head 应保留正文，实际: %q", head)
	}
}

// TestInlineSvgIsNotCountedAsImageReference 钉住：内联 SVG 不被当成图片引用。
//
// 引用账本从正文里解析 <img src> 建行。内联 SVG 不是 <img>，不该被计入
// 图片引用 —— 否则回收任务会以为某张图被引用了。
func TestInlineSvgIsNotCountedAsImageReference(t *testing.T) {
	safe := markdown.MdToSafe(svgFigureSource)

	urls := imageref.ParseMarkdownImageURLs(safe)

	if len(urls) != 1 || urls[0] != "https://cdn.example.com/a.png" {
		t.Fatalf("应只识别到 1 张图片语法的图，实际: %v", urls)
	}
}

// TestSvgSurvivesRoundTripForRendering 钉住入库消毒后可读回完整绘图元素，
// 保证前端第二层拿到的输入是完整的。
func TestSvgSurvivesRoundTripForRendering(t *testing.T) {
	safe := markdown.MdToSafe(svgFigureSource)

	for _, want := range []string{"<svg", "<title>", "<text>", "<rect", "<center>", `class="svg-block"`} {
		if !strings.Contains(safe, want) {
			t.Fatalf("入库消毒后应保留 %q: %s", want, safe)
		}
	}
	// 图注文字与正文也必须还在（不能被消毒误删）。
	for _, want := range []string{"图 1", "说明文字", "这是正文段落"} {
		if !strings.Contains(safe, want) {
			t.Fatalf("入库消毒后应保留文本 %q: %s", want, safe)
		}
	}
}
