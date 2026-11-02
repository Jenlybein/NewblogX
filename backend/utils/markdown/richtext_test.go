package markdown_test

import (
	"strings"
	"testing"

	"myblogx/utils/markdown"
)

// TestSanitizeRichTextHTMLStripsDangerousInput 钉住右栏富文本落库前的白名单：
// 脚本、内联事件、iframe 与伪协议都必须被剥离。
//
// 后端落库时清洗不是"读者站也会清洗"的重复劳动：库里存的是原始运营内容，
// 任何将来直接消费这段 HTML 的出口（邮件、导出、预览接口）都从这里受益。
func TestSanitizeRichTextHTMLStripsDangerousInput(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		banWords []string
	}{
		{
			name:     "script 标签",
			input:    `<p>正常</p><script>alert("x")</script>`,
			banWords: []string{"<script", "alert"},
		},
		{
			name:     "图片 onerror 事件",
			input:    `<img src="https://cdn.example.com/a.png" onerror="alert(1)">`,
			banWords: []string{"onerror", "alert"},
		},
		{
			name:     "javascript 伪协议",
			input:    `<a href="javascript:alert(1)">点我</a>`,
			banWords: []string{"javascript:", "alert"},
		},
		{
			name:     "data 伪协议",
			input:    `<a href="data:text/html;base64,PHNjcmlwdD4=">点我</a>`,
			banWords: []string{"data:", "base64"},
		},
		{
			name:     "iframe",
			input:    `<p>前</p><iframe src="https://evil.example.com"></iframe><p>后</p>`,
			banWords: []string{"iframe", "evil.example.com"},
		},
		{
			name:     "任意标签的 on 事件",
			input:    `<div onclick="alert(1)">文字</div>`,
			banWords: []string{"onclick", "alert"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := markdown.SanitizeRichTextHTML(tc.input)
			for _, word := range tc.banWords {
				if strings.Contains(strings.ToLower(got), strings.ToLower(word)) {
					t.Fatalf("恶意输入未被剥离：期望不含 %q，实际 %q（输入 %q）", word, got, tc.input)
				}
			}
		})
	}
}

// TestSanitizeRichTextHTMLKeepsFormatting 是"合法排版不被误杀"的对照：
// 后台只提供基础排版能力，这些标签必须原样活下来。
func TestSanitizeRichTextHTMLKeepsFormatting(t *testing.T) {
	input := `<h2>更新日志</h2><h3>本周改动</h3><p>本周 <strong>重点</strong> 与 <em>说明</em></p>` +
		`<ul><li>第一条</li><li>第二条</li></ul><ol><li>第一</li></ol>` +
		`<blockquote>引用一段</blockquote>` +
		`<p><a href="https://example.com/post">链接</a></p><pre><code>npm run dev</code></pre>`

	got := markdown.SanitizeRichTextHTML(input)

	for _, tag := range []string{"<h2>", "<h3>", "<strong>", "<em>", "<ul>", "<li>", "<ol>", "<blockquote>", "<pre>", "<code>"} {
		if !strings.Contains(got, tag) {
			t.Fatalf("合法排版被误杀：期望包含 %q，实际 %q", tag, got)
		}
	}
	for _, text := range []string{"https://example.com/post", "重点", "第一条"} {
		if !strings.Contains(got, text) {
			t.Fatalf("合法内容被误杀：期望包含 %q，实际 %q", text, got)
		}
	}
}

// TestSanitizeRichTextHTMLEmpty 空输入不制造占位内容（右栏据此隐藏整张卡）。
func TestSanitizeRichTextHTMLEmpty(t *testing.T) {
	if got := markdown.SanitizeRichTextHTML(""); got != "" {
		t.Fatalf("空输入应返回空串，实际 %q", got)
	}
	if got := markdown.SanitizeRichTextHTML("   "); strings.TrimSpace(got) != "" {
		t.Fatalf("纯空白输入应返回空串，实际 %q", got)
	}
}

// TestMdToHTMLSafeUnchanged 证明参数化没有把正文档的白名单收窄：
// 正文仍允许图片缩放这类富文本档用不上的历史放行项。
func TestMdToHTMLSafeUnchanged(t *testing.T) {
	md := `![图](https://cdn.example.com/a.png) 与 **重点**`
	got := markdown.MdToHTMLSafe(md)
	if !strings.Contains(got, "<img") || !strings.Contains(got, "<strong>") {
		t.Fatalf("正文档清洗不该被富文本档收窄：实际 %q", got)
	}
}
