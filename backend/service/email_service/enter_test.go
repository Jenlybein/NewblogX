package email_service

import (
	"errors"
	"myblogx/conf"
	"net/smtp"
	"testing"

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

func TestSendEmail_ReturnsWrappedErrorWhenSMTPFailed(t *testing.T) {
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
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装原始错误，实际=%v", err)
	}
}

func TestSendEmailFunctions(t *testing.T) {
	old := smtpSend
	defer func() { smtpSend = old }()

	smtpSend = func(_ *email.Email, _ string, _ smtp.Auth) error {
		return nil
	}

	emailConf := buildEmailConfig()
	if err := SendRegisterCode(emailConf, "u@example.com", "1234", 5); err != nil {
		t.Fatalf("SendRegisterCode 返回错误: %v", err)
	}
	if err := SendResetPwdCode(emailConf, "u@example.com", "1234", 5); err != nil {
		t.Fatalf("SendResetPwdCode 返回错误: %v", err)
	}
	if err := SendBindEmailCode(emailConf, "u@example.com", "1234", 5); err != nil {
		t.Fatalf("SendBindEmailCode 返回错误: %v", err)
	}
	if err := SendLoginCode(emailConf, "u@example.com", "1234", 5); err != nil {
		t.Fatalf("SendLoginCode 返回错误: %v", err)
	}
}
