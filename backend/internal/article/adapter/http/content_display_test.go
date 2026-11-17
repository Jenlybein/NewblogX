package http_test

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/platform/imagedisplay"
)

// contentPolicy 是只回答"能不能看"的判定替身。
type contentPolicy struct {
	visible map[string]bool
}

func (p contentPolicy) Visible(_ context.Context, urls []string) (map[string]bool, error) {
	return p.visible, nil
}

func (p contentPolicy) Sign(keyWithStyle string) string { return "signed:" + keyWithStyle }

func contextBackground() context.Context { return context.Background() }

// 本文件钉住 ADR-0034 里"正文内嵌图"这条接缝的两条要求：
//   - 读者看到的正文里，本站图片带样式后缀、未过审的换成占位；
//   - 原正文字段保持原图地址（编辑器回填不会把后缀写进库）。
//
// 用纯函数层验证而不是起 HTTP 服务：这里要证明的是"改写规则对不对"，
// 而"哪一屏用哪个字段"由前端契约决定（content_display 为空即回落 content）。

const (
	testMediaBase = "https://image.example.com"
	testImgA      = testMediaBase + "/myblogx/images/20261102/aaa"
	testImgB      = testMediaBase + "/myblogx/images/20261102/bbb"
)

// displayForContent 模拟 handler 里那一段：取 src → 批量派生 → 重写。
func displayForContent(html string, visible map[string]bool, style string) string {
	cfg := imagedisplay.Config{
		MediaBase:   testMediaBase,
		Prefix:      "myblogx",
		Delimiter:   "-",
		Styles:      map[imagedisplay.Purpose]string{imagedisplay.PurposeContent: style},
		GateEnabled: true,
	}
	resolver := imagedisplay.New(func() imagedisplay.Config { return cfg }, contentPolicy{visible: visible})

	srcs := imagedisplay.ExtractImgSrcs(html)
	items := make([]imagedisplay.Item, 0, len(srcs))
	for _, src := range srcs {
		items = append(items, imagedisplay.Item{URL: src, Purpose: imagedisplay.PurposeContent})
	}
	derived := imagedisplay.MustResolve(contextBackground(), resolver, imagedisplay.Viewer{}, items)

	resolved := make(map[string]string, len(srcs))
	for i, src := range srcs {
		resolved[src] = derived[i]
	}
	return imagedisplay.RewriteImgSrcs(html, resolved)
}

// TestContentDisplay_过审图带样式 钉住"读者正文里的图是压缩版"。
func TestContentDisplay_过审图带样式(t *testing.T) {
	t.Parallel()

	html := `<p>正文</p><img src="` + testImgA + `" alt="图">`
	out := displayForContent(html, map[string]bool{testImgA: true}, "content")

	if !strings.Contains(out, testImgA+"-content") {
		t.Fatalf("过审的图应带样式后缀，实际 %s", out)
	}
}

// TestContentDisplay_未过审换占位 钉住"读者看不到未过审的图"。
func TestContentDisplay_未过审换占位(t *testing.T) {
	t.Parallel()

	html := `<p>正文</p><img src="` + testImgA + `">`
	out := displayForContent(html, map[string]bool{testImgA: false}, "content")

	if strings.Contains(out, testImgA) {
		t.Fatalf("未过审的地址不该出现在读者正文里，实际 %s", out)
	}
	if !strings.Contains(out, "image-unavailable") {
		t.Fatalf("应替换成占位块，实际 %s", out)
	}
	if !strings.Contains(out, "<p>正文</p>") {
		t.Fatalf("正文其余部分不该被改动，实际 %s", out)
	}
}

// TestContentDisplay_一篇文章里两种图 钉住批量判定不会串味。
func TestContentDisplay_一篇文章里两种图(t *testing.T) {
	t.Parallel()

	html := `<img src="` + testImgA + `"><p>中间</p><img src="` + testImgB + `">`
	out := displayForContent(html, map[string]bool{testImgA: true, testImgB: false}, "content")

	if !strings.Contains(out, testImgA+"-content") {
		t.Fatalf("过审那张应带样式，实际 %s", out)
	}
	if strings.Contains(out, testImgB) {
		t.Fatalf("未过审那张不该出现，实际 %s", out)
	}
}

// TestContentDisplay_外链不改写 钉住"别人的图不加七牛样式后缀"。
func TestContentDisplay_外链不改写(t *testing.T) {
	t.Parallel()

	ext := "https://cdn.other.com/pic.png"
	html := `<img src="` + ext + `">`
	out := displayForContent(html, map[string]bool{}, "content")

	if !strings.Contains(out, ext) {
		t.Fatalf("外链应原样保留，实际 %s", out)
	}
}

// TestContentDisplay_原字段保持原图 钉住"编辑器回填不污染"。
//
// 这条是纯逻辑断言：派生只产出 content_display，原 content 字段在 handler 里
// 始终赋的是 a.Content()（原图地址），所以保存回库的永远是原图。
func TestContentDisplay_原字段保持原图(t *testing.T) {
	t.Parallel()

	html := `<img src="` + testImgA + `">`
	out := displayForContent(html, map[string]bool{testImgA: true}, "content")

	if !strings.Contains(html, testImgA) {
		t.Fatal("前提：原正文里就是原图地址")
	}
	if out == html {
		t.Fatal("派生后的正文应当不同于原正文（否则样式没生效）")
	}
	if strings.Contains(html, "-content") {
		t.Fatal("原正文不该被就地改写：派生返回新字符串")
	}
}
