package imagedisplay_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/platform/imagedisplay"
)

const (
	testBase   = "https://image.example.com"
	testPrefix = "myblogx"
	testKey    = "myblogx/images/20261102/abc123hash"
	testURL    = testBase + "/" + testKey
)

func newCfg(styles map[imagedisplay.Purpose]string) imagedisplay.Config {
	return imagedisplay.Config{
		MediaBase: testBase,
		Prefix:    testPrefix,
		Delimiter: "-",
		Styles:    styles,
		// 闸门默认开：这样测试里的 policy 才会真的被问到。
		GateEnabled: true,
	}
}

func fixed(cfg imagedisplay.Config) func() imagedisplay.Config {
	return func() imagedisplay.Config { return cfg }
}

// stubPolicy 是可编程的判定/签名替身。
type stubPolicy struct {
	visible    map[string]bool
	visibleErr error
	queried    []string
	signCalls  []string
}

func (p *stubPolicy) Visible(_ context.Context, urls []string) (map[string]bool, error) {
	p.queried = append(p.queried, urls...)
	if p.visibleErr != nil {
		return nil, p.visibleErr
	}
	return p.visible, nil
}

func (p *stubPolicy) Sign(keyWithStyle string) string {
	p.signCalls = append(p.signCalls, keyWithStyle)
	return "signed:" + keyWithStyle
}

// TestResolve_四档样式 钉住"用途 → 样式名"的映射与默认回落。
func TestResolve_四档样式(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{
		imagedisplay.PurposeCover:   "cover",
		imagedisplay.PurposeAvatar:  "avatar",
		imagedisplay.PurposeContent: "content",
		// chat 故意不配：未配置的档位必须用原图。
	})
	r := imagedisplay.New(fixed(cfg), nil)

	got, err := r.Resolve(context.Background(), imagedisplay.Viewer{}, []imagedisplay.Item{
		{URL: testURL, Purpose: imagedisplay.PurposeCover},
		{URL: testURL, Purpose: imagedisplay.PurposeAvatar},
		{URL: testURL, Purpose: imagedisplay.PurposeContent},
		{URL: testURL, Purpose: imagedisplay.PurposeChat},
	})
	if err != nil {
		t.Fatalf("Resolve 出错: %v", err)
	}

	want := []string{
		testURL + "-cover",
		testURL + "-avatar",
		testURL + "-content",
		testURL, // 未配置该档 → 原图
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 条 = %q，期望 %q", i, got[i], want[i])
		}
	}
}

// TestResolve_幂等 保证同一个地址派生两次不会长出两个后缀。
//
// 这条特别重要：读面可能对同一批地址反复派生（列表 + 详情 + 前端二次处理），
// 只要有一次是"在已派生的结果上再派生"，就会出现 `hash-small-small` 这种 404。
func TestResolve_幂等(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{imagedisplay.PurposeCover: "small"})
	r := imagedisplay.New(fixed(cfg), nil)

	first, _ := r.Resolve(context.Background(), imagedisplay.Viewer{}, []imagedisplay.Item{
		{URL: testURL, Purpose: imagedisplay.PurposeCover},
	})
	second, _ := r.Resolve(context.Background(), imagedisplay.Viewer{}, []imagedisplay.Item{
		{URL: first[0], Purpose: imagedisplay.PurposeCover},
	})

	if second[0] != first[0] {
		t.Fatalf("二次派生不应变化：第一次 %q，第二次 %q", first[0], second[0])
	}
}

// TestResolve_外链原样透传 保证不会给别人的图片地址加七牛样式后缀。
func TestResolve_外链原样透传(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{imagedisplay.PurposeCover: "small"})
	r := imagedisplay.New(fixed(cfg), nil)

	items := []imagedisplay.Item{
		{URL: "https://other.cdn.com/pic.jpg", Purpose: imagedisplay.PurposeCover},
		{URL: "https://image.example.com/uploads/local.png", Purpose: imagedisplay.PurposeCover},
		{URL: "data:image/png;base64,AAAA", Purpose: imagedisplay.PurposeCover},
		{URL: "", Purpose: imagedisplay.PurposeCover},
	}
	got, err := r.Resolve(context.Background(), imagedisplay.Viewer{}, items)
	if err != nil {
		t.Fatalf("Resolve 出错: %v", err)
	}
	for i, item := range items {
		if got[i] != item.URL {
			t.Fatalf("第 %d 条（%q）被改写成了 %q", i, item.URL, got[i])
		}
	}
}

// TestResolve_闸门 钉住"未过审只有管理员能看、判定失败一律不展示"。
func TestResolve_闸门(t *testing.T) {
	t.Parallel()

	cfg := newCfg(nil)

	t.Run("未过审对普通观看者返回空", func(t *testing.T) {
		t.Parallel()
		r := imagedisplay.New(fixed(cfg), &stubPolicy{visible: map[string]bool{testURL: false}})
		got, err := r.Resolve(context.Background(), imagedisplay.Viewer{},
			[]imagedisplay.Item{{URL: testURL, Purpose: imagedisplay.PurposeCover}})
		if err != nil {
			t.Fatalf("Resolve 出错: %v", err)
		}
		if got[0] != "" {
			t.Fatalf("未过审的图不应给出地址，实际 %q", got[0])
		}
	})

	t.Run("过审后正常返回", func(t *testing.T) {
		t.Parallel()
		r := imagedisplay.New(fixed(cfg), &stubPolicy{visible: map[string]bool{testURL: true}})
		got, _ := r.Resolve(context.Background(), imagedisplay.Viewer{},
			[]imagedisplay.Item{{URL: testURL, Purpose: imagedisplay.PurposeCover}})
		if got[0] != testURL {
			t.Fatalf("过审后应返回地址，实际 %q", got[0])
		}
	})

	t.Run("管理员绕过闸门", func(t *testing.T) {
		t.Parallel()
		p := &stubPolicy{visible: map[string]bool{testURL: false}}
		r := imagedisplay.New(fixed(cfg), p)
		got, _ := r.Resolve(context.Background(), imagedisplay.Viewer{IsAdmin: true},
			[]imagedisplay.Item{{URL: testURL, Purpose: imagedisplay.PurposeCover}})
		if got[0] != testURL {
			t.Fatalf("管理员应看到原图，实际 %q", got[0])
		}
		if len(p.queried) != 0 {
			t.Fatalf("管理员不应触发状态查询，实际查了 %v", p.queried)
		}
	})

	t.Run("判定失败一律不展示", func(t *testing.T) {
		t.Parallel()
		r := imagedisplay.New(fixed(cfg), &stubPolicy{visibleErr: errors.New("db down")})
		got, err := r.Resolve(context.Background(), imagedisplay.Viewer{},
			[]imagedisplay.Item{{URL: testURL, Purpose: imagedisplay.PurposeCover}})
		if err == nil {
			t.Fatal("判定失败应返回错误")
		}
		if got[0] != "" {
			t.Fatalf("判定失败时不应给出地址，实际 %q", got[0])
		}
	})
}

// TestResolve_私有读签名顺序 钉住"先拼样式再签名"。
//
// 七牛的私有空间下载凭证是按完整 URL 计算的。先对原图签名再拼 `-style`，
// 令牌就失效了 —— 这个顺序错了表现为"图片全部 401"，而且看不出原因。
func TestResolve_私有读签名顺序(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{imagedisplay.PurposeCover: "small"})
	cfg.PrivateRead = true
	// 过审后才谈得上"签名"：不可见的图连地址都不给，自然不会走到签名这一步。
	p := &stubPolicy{visible: map[string]bool{testURL: true}}
	r := imagedisplay.New(fixed(cfg), p)

	got, err := r.Resolve(context.Background(), imagedisplay.Viewer{},
		[]imagedisplay.Item{{URL: testURL, Purpose: imagedisplay.PurposeCover}})
	if err != nil {
		t.Fatalf("Resolve 出错: %v", err)
	}

	wantKey := testKey + "-small"
	if len(p.signCalls) != 1 || p.signCalls[0] != wantKey {
		t.Fatalf("签名入参应为已拼样式的 key %q，实际 %v", wantKey, p.signCalls)
	}
	if got[0] != "signed:"+wantKey {
		t.Fatalf("应返回签名结果，实际 %q", got[0])
	}
}

// TestConfig_ObjectKey 钉住"哪些地址算本站图片"的判定口径。
func TestConfig_ObjectKey(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{imagedisplay.PurposeCover: "small"})

	cases := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{name: "标准地址", raw: testURL, want: testKey, wantOK: true},
		{name: "带样式后缀", raw: testURL + "-small", want: testKey, wantOK: true},
		{name: "带查询参数", raw: testURL + "?e=1&token=x", want: testKey, wantOK: true},
		{name: "相对路径", raw: "/" + testKey, want: testKey, wantOK: true},
		{name: "别的域名", raw: "https://other.cdn.com/" + testKey, wantOK: false},
		{name: "本站但非图片前缀", raw: testBase + "/uploads/a.png", wantOK: false},
		{name: "空地址", raw: "", wantOK: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := cfg.ObjectKey(c.raw)
			if ok != c.wantOK {
				t.Fatalf("ok = %v，期望 %v", ok, c.wantOK)
			}
			if ok && got != c.want {
				t.Fatalf("key = %q，期望 %q", got, c.want)
			}
		})
	}
}

// TestConfig_WithStyle_不误伤历史 key 保证"剥后缀"只认已配置的样式名。
//
// 对象 key 是哈希，但历史数据里可能有带连字符的文件名；随便剥任意后缀
// 会让这类图变成 404 或串到别的图。
func TestConfig_WithStyle_不误伤历史key(t *testing.T) {
	t.Parallel()

	cfg := newCfg(map[imagedisplay.Purpose]string{imagedisplay.PurposeCover: "small"})
	key := "myblogx/images/20261102/my-old-photo"

	if got := cfg.WithStyle(key, "small"); got != key+"-small" {
		t.Fatalf("应追加样式，实际 %q", got)
	}
	// 未配置的样式名不应被当成"已有后缀"剥离。
	if got := cfg.WithStyle(key+"-big", "small"); got != key+"-big-small" {
		t.Fatalf("未配置的后缀不应被剥离，实际 %q", got)
	}
}

