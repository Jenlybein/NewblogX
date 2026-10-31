package app

import (
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/apperr"
	"myblogx/internal/platform/mail"
	redis_service "myblogx/internal/platform/redis"
	userapptest "myblogx/internal/user/apptest"
	userdomain "myblogx/internal/user/domain"
)

// 本文件锁死 emailCodeBridge.Send 的一条约定：
//
//	邮箱状态与验证码用途不匹配时，**必须返回失败并说明原因**，不能"返回成功但不发信"。
//
// 2026-10-31 线上事故就是后者：读者站注册表单传了 2（重置密码），而目标邮箱从未注册，
// 旧实现照旧返回 `code:0` + 一个 id，前端提示"验证码已发送"，用户却永远收不到信；
// 拿到的 id 在 Redis 里没有对应记录，下一步只会得到"验证码不存在或错误"。
// 决策与取舍见 docs/adr/0014。

// recordingSender 记录真的发出去的模板邮件。
type recordingSender struct {
	sent []mail.TemplateMessage
}

// SendTemplate 实现 mail.Sender。
func (s *recordingSender) SendTemplate(_ context.Context, _ conf.Mail, msg mail.TemplateMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

// newTestEmailCodeBridge 用内存仓储 + 录制发送器装配被测对象。
func newTestEmailCodeBridge(users userdomain.Repository, sender *recordingSender) *emailCodeBridge {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &emailCodeBridge{
		users:  users,
		mailer: mail.NewWithSender(sender),
		mailConf: func() conf.Mail {
			return conf.Mail{
				SecretID:    "id",
				SecretKey:   "key",
				Region:      "ap-guangzhou",
				FromAddress: "noreply@example.com",
				FromName:    "果冻盒",
				TemplateID:  100,
			}
		},
		// 没有 Redis 时 AllowEmailSend 直接放行（redis_auth.go 的 fail-open），
		// 频控不在本文件的验证范围内。
		redisOptions: redis_service.Options{},
		loginConf: func() siteconf.Login {
			return siteconf.Login{
				EmailCodeTimeout:      5,
				EmailSendWindowSecond: 60,
				EmailSendPerEmailMax:  1,
				EmailSendPerIPMax:     10,
			}
		},
		logger: logger,
	}
}

// TestEmailCodeRefusesMismatchedPurpose 覆盖四个"状态与用途不匹配"的分支。
func TestEmailCodeRefusesMismatchedPurpose(t *testing.T) {
	t.Parallel()

	const target = "someone@example.com"
	cases := []struct {
		name   string
		kind   mail.Kind
		exists bool
		want   apperr.Code
	}{
		{name: "注册但邮箱已注册", kind: mail.KindRegister, exists: true, want: apperr.CodeUserEmailRegistered},
		{name: "绑定邮箱但邮箱已被占用", kind: mail.KindBindEmail, exists: true, want: apperr.CodeUserEmailTaken},
		{name: "重置密码但邮箱未注册", kind: mail.KindResetPassword, want: apperr.CodeUserEmailUnregistered},
		{name: "邮箱登录但邮箱未注册", kind: mail.KindLogin, want: apperr.CodeUserEmailUnregistered},
		{name: "用途不在 1-4 之内", kind: mail.Kind(0), want: apperr.CodeUserMailSendFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			users := userapptest.NewUsers()
			if tc.exists {
				email := target
				users.Seed(userdomain.User{Username: "u1", Email: &email})
			}
			sender := &recordingSender{}
			bridge := newTestEmailCodeBridge(users, sender)

			id, err := bridge.Send(context.Background(), int8(tc.kind), target, "1.2.3.4")

			if id != "" {
				t.Fatalf("被拒时不应返回验证码记录 ID，实际 %q", id)
			}
			code, ok := apperr.CodeOf(err)
			if !ok || code != tc.want {
				t.Fatalf("错误码应为 %s，实际 %v", tc.want, err)
			}
			if len(sender.sent) != 0 {
				t.Fatalf("被拒时不应发信，实际发了 %d 封", len(sender.sent))
			}
		})
	}
}

// TestEmailCodeSendsWhenPurposeMatches 锁住另一半：状态匹配时信真的发出去。
func TestEmailCodeSendsWhenPurposeMatches(t *testing.T) {
	t.Parallel()

	const target = "someone@example.com"
	users := userapptest.NewUsers()
	sender := &recordingSender{}
	bridge := newTestEmailCodeBridge(users, sender)

	// 测试环境没有 Redis：发信成功之后那一步 Store 必然失败，所以这里不要求
	// err == nil，只要求"信确实发了"——这正是旧实现在这个分支上缺的东西
	// （旧实现一封信都不发，却返回成功）。
	if _, err := bridge.Send(context.Background(), int8(mail.KindRegister), target, "1.2.3.4"); err == nil {
		t.Fatal("没有 Redis 时验证码存储应失败")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("应发出 1 封信，实际 %d 封", len(sender.sent))
	}

	msg := sender.sent[0]
	if msg.To != target {
		t.Fatalf("收件人应为 %s，实际 %s", target, msg.To)
	}
	if msg.Vars["action"] != "账号注册" {
		t.Fatalf("模板动作变量应为「账号注册」，实际 %q", msg.Vars["action"])
	}
	if msg.Vars["code"] == "" {
		t.Fatal("模板里必须带验证码")
	}
	if msg.Subject != "【果冻盒】 账号注册" {
		t.Fatalf("邮件主题不符: %q", msg.Subject)
	}
}
