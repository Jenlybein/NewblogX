package mail

import (
	"context"
	"errors"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"
)

// fakeSender 记录被发出的邮件，并可按需返回错误。
//
// 用注入而不是包级可变变量：旧实现改的是 `var smtpSend`，
// 并行测试里会互相干扰。
type fakeSender struct {
	calls []sentCall
	err   error
}

type sentCall struct {
	cfg conf.Mail
	msg TemplateMessage
}

func (f *fakeSender) SendTemplate(_ context.Context, cfg conf.Mail, msg TemplateMessage) error {
	f.calls = append(f.calls, sentCall{cfg: cfg, msg: msg})
	return f.err
}

// usableConfig 返回一份"配好了"的邮件配置。
func usableConfig() conf.Mail {
	return conf.Mail{
		SecretID:    "id",
		SecretKey:   "key",
		Region:      "ap-guangzhou",
		FromAddress: "noreply@example.com",
		FromName:    "BlogX",
		TemplateID:  1234,
	}
}

func TestSendCode_四种场景的主题与变量(t *testing.T) {
	fake := &fakeSender{}
	svc := NewWithSender(fake)
	cfg := usableConfig()

	cases := []struct {
		kind   Kind
		action string
	}{
		{KindRegister, "账号注册"},
		{KindResetPassword, "重置密码"},
		{KindBindEmail, "绑定邮箱"},
		{KindLogin, "邮箱登录"},
	}
	for _, c := range cases {
		if err := svc.SendCode(context.Background(), cfg, "u@example.com", c.kind, "1234", 5); err != nil {
			t.Fatalf("%s 返回错误: %v", c.action, err)
		}
	}

	if len(fake.calls) != len(cases) {
		t.Fatalf("期望发出 %d 封，实际 %d 封", len(cases), len(fake.calls))
	}
	// 四个场景共用同一个模板，靠 action 变量区分 —— 变量错了用户会看到
	// "您正在进行账号注册操作"却是在重置密码。
	for i, c := range cases {
		got := fake.calls[i].msg
		if want := "【BlogX】 " + c.action; got.Subject != want {
			t.Fatalf("第 %d 封主题期望 %q，实际 %q", i, want, got.Subject)
		}
		if got.Vars["action"] != c.action {
			t.Fatalf("第 %d 封的 action 变量期望 %q，实际 %q", i, c.action, got.Vars["action"])
		}
		if got.Vars["code"] != "1234" {
			t.Fatalf("第 %d 封的 code 变量错误: %q", i, got.Vars["code"])
		}
		if got.Vars["timeout"] != "5" {
			t.Fatalf("第 %d 封的 timeout 变量错误: %q", i, got.Vars["timeout"])
		}
		if got.To != "u@example.com" {
			t.Fatalf("第 %d 封收件人错误: %q", i, got.To)
		}
	}
}

func TestSendCode_发送失败时保留错误链与错误码(t *testing.T) {
	sentinel := errors.New("ses unavailable")
	svc := NewWithSender(&fakeSender{err: sentinel})

	err := svc.SendCode(context.Background(), usableConfig(), "u@example.com", KindLogin, "1234", 5)
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	// 错误链必须能穿到底层原因：日志要能定位是密钥错还是模板不存在。
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装原始错误，实际=%v", err)
	}
	// 错误码必须是指定的那个：调用方靠它决定给用户看什么文案。
	if !apperr.IsCode(err, apperr.CodeUserMailSendFailed) {
		t.Fatalf("期望错误码 user.mail_send_failed，实际=%v", err)
	}
}

func TestSendCode_配置不全时不调用外部接口(t *testing.T) {
	// 这些条件不满足还去调 SES，只会拿到 InvalidParameter 之类的错误，
	// 日志里看不出"没配"还是"配错了"。必须在本地就拒掉。
	cases := []struct {
		name   string
		mutate func(*conf.Mail)
		to     string
	}{
		{"没填 SecretID", func(c *conf.Mail) { c.SecretID = "" }, "u@example.com"},
		{"没填 SecretKey", func(c *conf.Mail) { c.SecretKey = "" }, "u@example.com"},
		{"没填发信地址", func(c *conf.Mail) { c.FromAddress = "" }, "u@example.com"},
		{"模板 ID 为 0", func(c *conf.Mail) { c.TemplateID = 0 }, "u@example.com"},
		{"密钥只有空格", func(c *conf.Mail) { c.SecretID = "   " }, "u@example.com"},
		{"收件人为空", func(*conf.Mail) {}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeSender{}
			svc := NewWithSender(fake)
			cfg := usableConfig()
			c.mutate(&cfg)

			err := svc.SendCode(context.Background(), cfg, c.to, KindLogin, "1234", 5)
			if !apperr.IsCode(err, apperr.CodeUserMailSendFailed) {
				t.Fatalf("期望 user.mail_send_failed，实际=%v", err)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("配置不全时不该发起外部调用，实际发了 %d 封", len(fake.calls))
			}
		})
	}
}

func TestSendCode_未知场景直接失败(t *testing.T) {
	fake := &fakeSender{}
	svc := NewWithSender(fake)

	if err := svc.SendCode(context.Background(), usableConfig(), "u@example.com", Kind(99), "1234", 5); err == nil {
		t.Fatal("未知场景应返回错误")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("未知场景不该发信，实际发了 %d 封", len(fake.calls))
	}
}

func TestSendCode_timeout非法时兜底(t *testing.T) {
	// 运行时配置里 email_code_timeout 没填时是 0，正文不能印"有效期 0 分钟"。
	for _, timeout := range []int{0, -3} {
		fake := &fakeSender{}
		svc := NewWithSender(fake)
		if err := svc.SendCode(context.Background(), usableConfig(), "u@example.com", KindLogin, "1234", timeout); err != nil {
			t.Fatalf("timeout=%d 返回错误: %v", timeout, err)
		}
		if got := fake.calls[0].msg.Vars["timeout"]; got != "5" {
			t.Fatalf("timeout=%d 时应兜底为 5，实际 %q", timeout, got)
		}
	}
}

func TestSubjectOf_发件人显示名留空时只给动作(t *testing.T) {
	if got := subjectOf(conf.Mail{}, "邮箱登录"); got != "邮箱登录" {
		t.Fatalf("FromName 为空时主题不该出现空括号，实际 %q", got)
	}
	if got := subjectOf(conf.Mail{FromName: "  "}, "邮箱登录"); got != "邮箱登录" {
		t.Fatalf("FromName 只有空格时同样不该出现空括号，实际 %q", got)
	}
}

func TestFromAddressOf_SES要求的别名格式(t *testing.T) {
	// 官方格式：别名 + 一个空格 + <邮箱地址>，别名里不能有冒号。
	got := fromAddressOf(conf.Mail{FromName: "BlogX", FromAddress: "noreply@example.com"})
	if want := "BlogX <noreply@example.com>"; got != want {
		t.Fatalf("期望 %q，实际 %q", want, got)
	}
	// 没给显示名时只发地址：拼成 " <addr>" 会被 SES 判为非法发信地址。
	if got := fromAddressOf(conf.Mail{FromAddress: "noreply@example.com"}); got != "noreply@example.com" {
		t.Fatalf("FromName 为空时只该给地址，实际 %q", got)
	}
}

func TestNewSESClient_超时与地域兜底(t *testing.T) {
	// 超时不设就等于用 SDK 默认的 60 秒，会把登录请求拖到分钟级。
	client, err := newSESClient(conf.Mail{SecretID: "id", SecretKey: "key"})
	if err != nil {
		t.Fatalf("构造客户端失败: %v", err)
	}
	if client.GetRegion() != DefaultRegion {
		t.Fatalf("Region 留空时应兜底为 %q，实际 %q", DefaultRegion, client.GetRegion())
	}

	custom, err := newSESClient(conf.Mail{
		SecretID: "id", SecretKey: "key",
		Region: "ap-hongkong", Endpoint: "ses.intl.tencentcloudapi.com",
	})
	if err != nil {
		t.Fatalf("构造客户端失败: %v", err)
	}
	if custom.GetRegion() != "ap-hongkong" {
		t.Fatalf("显式 Region 应生效，实际 %q", custom.GetRegion())
	}
}
