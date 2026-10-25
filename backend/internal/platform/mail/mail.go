// Package mail 发送账号相关的验证码邮件与普通邮件。
//
// 这是纯技术能力（SMTP 发送 + 文案模板），没有业务状态，
// 所以放 platform 而不是 service。调用方只传配置和收件信息。
package mail

import (
	"fmt"
	"net/smtp"

	"myblogx/conf"
	"myblogx/internal/apperr"

	"github.com/jordan-wright/email"
)

// smtpSend 是可替换的发送函数，测试用它注入失败的 SMTP。
//
// 注意：包级可变变量在并行测试里不安全。保留它是为了不改动既有测试的
// mock 方式；如果后续要并行化，应改成 Service 结构体的字段。
var smtpSend = func(e *email.Email, addr string, auth smtp.Auth) error {
	return e.Send(addr, auth)
}

// SendRegisterCode 发送注册验证码。
func SendRegisterCode(config conf.Email, to string, code string, timeout int) error {
	return sendCode(config, to, "账号注册", code, timeout)
}

// SendResetPwdCode 发送重置密码验证码。
func SendResetPwdCode(config conf.Email, to string, code string, timeout int) error {
	return sendCode(config, to, "重置密码", code, timeout)
}

// SendBindEmailCode 发送绑定邮箱验证码。
func SendBindEmailCode(config conf.Email, to string, code string, timeout int) error {
	return sendCode(config, to, "绑定邮箱", code, timeout)
}

// SendLoginCode 发送邮箱登录验证码。
func SendLoginCode(config conf.Email, to string, code string, timeout int) error {
	return sendCode(config, to, "邮箱登录", code, timeout)
}

// sendCode 拼接验证码正文并发送。
//
// 四种场景的文案只差操作名，合并成一个函数避免四份复制后改一处漏三处。
func sendCode(config conf.Email, to, action, code string, timeout int) error {
	subject := fmt.Sprintf("【%s】 %s", config.SendNickname, action)
	text := fmt.Sprintf("您正在进行%s操作，这是您的验证码为：%s，有效期为 %d 分钟", action, code, timeout)
	return SendEmail(config, to, subject, text)
}

// SendEmail 发送一封纯文本邮件。
func SendEmail(config conf.Email, to string, subject string, text string) error {
	e := &email.Email{
		From:    fmt.Sprintf("%s <%s>", config.SendNickname, config.SendEmail),
		To:      []string{to},
		Subject: subject,
		Text:    []byte(text),
	}
	addr := fmt.Sprintf("%s:%d", config.Domain, config.Port)
	if err := smtpSend(e, addr, smtp.PlainAuth("",
		config.SendEmail,
		config.AuthCode,
		config.Domain,
	)); err != nil {
		// 错误码交给调用方翻译；底层原因用 Wrap 保留，便于日志定位 SMTP 问题。
		return apperr.Wrap(apperr.CodeUserMailSendFailed, err)
	}
	return nil
}
