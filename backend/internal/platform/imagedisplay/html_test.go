package imagedisplay_test

import (
	"strings"
	"testing"

	"myblogx/internal/platform/imagedisplay"
)

// TestExtractImgSrcs 钉住"能从正文里按顺序找出所有图片地址"。
func TestExtractImgSrcs(t *testing.T) {
	t.Parallel()

	html := `<p>前</p><img src="https://a/1.png" alt="x"><img class="k" src='https://a/2.png'>` +
		`<img src="https://a/3.png"><p>后</p>`

	got := imagedisplay.ExtractImgSrcs(html)
	want := []string{"https://a/1.png", "https://a/2.png", "https://a/3.png"}
	if len(got) != len(want) {
		t.Fatalf("提取 %d 条，期望 %d 条: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 条 = %q，期望 %q", i, got[i], want[i])
		}
	}
}

// TestExtractImgSrcs_忽略伪标签 保证不会把 <image-x> 当成图片。
func TestExtractImgSrcs_忽略伪标签(t *testing.T) {
	t.Parallel()

	html := `<image-slot src="https://a/1.png"></image-slot><img src="https://a/2.png">`
	got := imagedisplay.ExtractImgSrcs(html)
	if len(got) != 1 || got[0] != "https://a/2.png" {
		t.Fatalf("应只提取真正的 img，实际 %v", got)
	}
}

// TestRewriteImgSrcs 钉住三种语义：替换 / 屏蔽 / 原样保留。
func TestRewriteImgSrcs(t *testing.T) {
	t.Parallel()

	html := `<p>文</p>` +
		`<img src="https://image.example.com/myblogx/images/1/a.png" alt="图一">` +
		`<img src="https://cdn.other.com/x.png">` +
		`<img src="https://image.example.com/myblogx/images/2/b.png">` +
		`<p>尾</p>`

	resolved := map[string]string{
		"https://image.example.com/myblogx/images/1/a.png": "https://image.example.com/myblogx/images/1/a.png-content",
		"https://image.example.com/myblogx/images/2/b.png": "", // 未过审 → 占位
		// 外链不在映射里 → 原样保留
	}

	out := imagedisplay.RewriteImgSrcs(html, resolved)

	if !strings.Contains(out, "a.png-content") {
		t.Fatalf("应替换成样式地址，实际 %s", out)
	}
	if !strings.Contains(out, `alt="图一"`) {
		t.Fatalf("替换 src 时不应丢其余属性，实际 %s", out)
	}
	if !strings.Contains(out, "https://cdn.other.com/x.png") {
		t.Fatalf("外链必须原样保留，实际 %s", out)
	}
	if !strings.Contains(out, "image-unavailable") {
		t.Fatalf("未过审的图应替换成占位块，实际 %s", out)
	}
	if strings.Contains(out, "images/2/b.png") {
		t.Fatalf("未过审的地址不应留在输出里，实际 %s", out)
	}
	if !strings.Contains(out, "<p>文</p>") || !strings.Contains(out, "<p>尾</p>") {
		t.Fatalf("正文其余部分不应被改动，实际 %s", out)
	}
}

// TestRewriteImgSrcs_空映射原样返回 保证没有图片时零成本。
func TestRewriteImgSrcs_空映射原样返回(t *testing.T) {
	t.Parallel()

	html := `<p>没有图片</p>`
	if got := imagedisplay.RewriteImgSrcs(html, nil); got != html {
		t.Fatalf("无 img 时应原样返回，实际 %s", got)
	}
}

// TestRewriteImgSrcs_单引号与无引号 保证重写不依赖引号风格。
func TestRewriteImgSrcs_单引号与无引号(t *testing.T) {
	t.Parallel()

	single := `<img src='https://x/1.png'>`
	if out := imagedisplay.RewriteImgSrcs(single, map[string]string{"https://x/1.png": "https://x/2.png"}); !strings.Contains(out, `'https://x/2.png'`) {
		t.Fatalf("单引号应保持单引号，实际 %s", out)
	}

	bare := `<img src=https://x/1.png>`
	if out := imagedisplay.RewriteImgSrcs(bare, map[string]string{"https://x/1.png": "https://x/2.png"}); !strings.Contains(out, "https://x/2.png") {
		t.Fatalf("无引号也应能替换，实际 %s", out)
	}
}
