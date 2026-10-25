package mail

import (
	"errors"
	"net/smtp"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"

	"github.com/jordan-wright/email"
)

func buildEmailConfig() conf.Email {
	return conf.Email{
		Domain:       "smtp.example.com",
		Port:         25,
		SendEmail:    "noreply@example.com",
		AuthCode:     "x",
		SendNickname: "BlogX",
	}
}

func TestSendEmailReturnsWrappedErrorWhenSMTPFailed(t *testing.T) {
	old := smtpSend
	defer func() { smtpSend = old }()

	sentinel := errors.New("smtp unavailable")
	smtpSend = func(_ *email.Email, _ string, _ smtp.Auth) error {
		return sentinel
	}

	err := SendEmail(buildEmailConfig(), "u@example.com", "主题", "正文")
	if err == nil {
		t.Fatalf("期望返回错误，实际为 nil")
	}
	// 错误链必须能穿到底层原因：日志要能定位 SMTP 的具体失败。
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装原始错误，实际=%v", err)
	}
	// 错误码必须是指定的那个：调用方靠它决定给用户看什么文案。
	if !apperr.IsCode(err, apperr.CodeUserMailSendFailed) {
		t.Fatalf("期望错误码 user.mail_send_failed，实际=%v", err)
	}
}

func TestSendCodeFunctions(t *testing.T) {
	old := smtpSend
	defer func() { smtpSend = old }()

	var subjects []string
	smtpSend = func(e *email.Email, _ string, _ smtp.Auth) error {
		subjects = append(subjects, e.Subject)
		return nil
	}

	emailConf := buildEmailConfig()
	cases := []struct {
		name string
		send func() error
		want string
	}{
		{"注册", func() error { return SendRegisterCode(emailConf, "u@example.com", "1234", 5) }, "【BlogX】 账号注册"},
		{"重置密码", func() error { return SendResetPwdCode(emailConf, "u@example.com", "1234", 5) }, "【BlogX】 重置密码"},
		{"绑定邮箱", func() error { return SendBindEmailCode(emailConf, "u@example.com", "1234", 5) }, "【BlogX】 绑定邮箱"},
		{"邮箱登录", func() error { return SendLoginCode(emailConf, "u@example.com", "1234", 5) }, "【BlogX】 邮箱登录"},
	}
	for _, c := range cases {
		if err := c.send(); err != nil {
			t.Fatalf("%s 返回错误: %v", c.name, err)
		}
	}
	// 合并成 sendCode 之后，四种场景的标题不能被改错 —— 用户靠标题区分自己在做什么。
	for i, want := range []string{"【BlogX】 账号注册", "【BlogX】 重置密码", "【BlogX】 绑定邮箱", "【BlogX】 邮箱登录"} {
		if i >= len(subjects) || subjects[i] != want {
			t.Fatalf("第 %d 封邮件标题期望 %q，实际 %q（全部=%v）", i, want, safeIndex(subjects, i), subjects)
		}
	}
}

func safeIndex(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}
