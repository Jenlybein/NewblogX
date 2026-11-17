package app_test

import (
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/site/app"
)

// validStorage 返回一份合法基线，用例只改关心的字段。
func validStorage() conf.QiNiu {
	return conf.QiNiu{
		AccessKey:      "ak",
		SecretKey:      "sk",
		ImageStyleCover: "cover",
		StyleDelimiter: "-",
		DownloadExpiry: 3600,
		AuditTimeoutSeconds: 60,
	}
}

// TestNormalizeStorage_样式名校验 钉住"坏样式名必须当场拒绝"。
//
// 样式名会进 URL 路径。填错了七牛不会回退原图、也不会报错，只会让这个槽位的
// 图片全部 404 —— 所以在保存期就要拒掉不可能成立的写法。
func TestNormalizeStorage_样式名校验(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*conf.QiNiu)
		wantErr bool
	}{
		{name: "合法样式名", mutate: func(c *conf.QiNiu) { c.ImageStyleCover = "cover" }},
		{name: "空样式名表示用原图", mutate: func(c *conf.QiNiu) { c.ImageStyleCover = "" }},
		{name: "带空格", mutate: func(c *conf.QiNiu) { c.ImageStyleCover = "my style" }, wantErr: true},
		{name: "带斜杠", mutate: func(c *conf.QiNiu) { c.ImageStyleAvatar = "a/b" }, wantErr: true},
		{name: "带问号", mutate: func(c *conf.QiNiu) { c.ImageStyleChat = "a?b" }, wantErr: true},
		{name: "超长", mutate: func(c *conf.QiNiu) { c.ImageStyleContent = strings.Repeat("a", 33) }, wantErr: true},
		{name: "允许减号下划线", mutate: func(c *conf.QiNiu) { c.ImageStyleCover = "thumb_800-2x" }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := validStorage()
			c.mutate(&in)
			_, err := app.NormalizeStorage(in)
			if c.wantErr && err == nil {
				t.Fatal("应当拒绝，实际通过了")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("应当通过，实际 %v", err)
			}
		})
	}
}

// TestNormalizeStorage_分隔符 只放行不会破坏 URL 结构的单字符。
//
// `/` 会把样式后缀变成路径分隔、`?` `#` 会截断 URL —— 这些放进来的表现是
// "图片地址看起来对但打不开"，而且很难从错误信息反推。
func TestNormalizeStorage_分隔符(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value   string
		want    string
		wantErr bool
	}{
		{value: "", want: "-"},
		{value: "-", want: "-"},
		{value: "_", want: "_"},
		{value: "!", want: "!"},
		{value: "/", wantErr: true},
		{value: "?", wantErr: true},
		{value: "#", wantErr: true},
		{value: "ab", wantErr: true},
	}

	for _, c := range cases {
		t.Run("分隔符="+c.value, func(t *testing.T) {
			t.Parallel()
			in := validStorage()
			in.StyleDelimiter = c.value
			out, err := app.NormalizeStorage(in)
			if c.wantErr {
				if err == nil {
					t.Fatal("应当拒绝，实际通过了")
				}
				return
			}
			if err != nil {
				t.Fatalf("应当通过，实际 %v", err)
			}
			if out.StyleDelimiter != c.want {
				t.Fatalf("分隔符 = %q，期望 %q", out.StyleDelimiter, c.want)
			}
		})
	}
}

// TestNormalizeStorage_签名有效期与审核超时 钉住"没填回落、越界拒绝"。
func TestNormalizeStorage_签名有效期与审核超时(t *testing.T) {
	t.Parallel()

	in := validStorage()
	in.DownloadExpiry = 0
	in.AuditTimeoutSeconds = 0
	out, err := app.NormalizeStorage(in)
	if err != nil {
		t.Fatalf("没填应回落默认值，实际 %v", err)
	}
	if out.DownloadExpiry == 0 || out.AuditTimeoutSeconds == 0 {
		t.Fatalf("默认值未补齐: %+v", out)
	}

	in = validStorage()
	in.DownloadExpiry = 10
	if _, err := app.NormalizeStorage(in); err == nil {
		t.Fatal("下载签名有效期过短应被拒绝")
	}

	in = validStorage()
	in.AuditTimeoutSeconds = 100000
	if _, err := app.NormalizeStorage(in); err == nil {
		t.Fatal("审核超时过长应被拒绝")
	}
}

// TestNormalizeStorage_私有空间必须有密钥 钉住"签不出令牌的配置不许保存"。
func TestNormalizeStorage_私有空间必须有密钥(t *testing.T) {
	t.Parallel()

	in := validStorage()
	in.PrivateRead = true
	in.AccessKey = ""
	if _, err := app.NormalizeStorage(in); err == nil {
		t.Fatal("私有空间缺 AccessKey 应被拒绝")
	}

	in = validStorage()
	in.PrivateRead = true
	in.SecretKey = ""
	if _, err := app.NormalizeStorage(in); err == nil {
		t.Fatal("私有空间缺 SecretKey 应被拒绝")
	}

	in = validStorage()
	in.PrivateRead = true
	if _, err := app.NormalizeStorage(in); err != nil {
		t.Fatalf("私有空间密钥齐备应当通过，实际 %v", err)
	}
}
