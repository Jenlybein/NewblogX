// Package sms 发送与核验手机验证码。
//
// 底层是阿里云**号码认证服务（PNVS）的短信认证**，用官方 Go SDK
// github.com/alibabacloud-go/dypnsapi-20170525/v2 调用，不手写请求签名。
//
// # 一个必须记住的事实：验证码不是我们生成的
//
// PNVS 的 SendSmsVerifyCode 没有"自定义验证码内容"的参数（SDK 里只有
// CodeLength / CodeType / ValidTime 这些**生成规则**），验证码由平台生成并
// 随模板下发，核验必须调 CheckSmsVerifyCode。所以本项目**没有**自己的验证码
// 存储（不存在 redis_sms 这类东西）：
//
//   - 发送：调 Send，平台记下 (手机号, 验证码, 有效期)
//   - 核验：调 Check，平台回答 PASS / 其他
//
// 好处是验证码与失败次数不需要我们维护；代价是每次登录多一次外部调用，
// 且"验证码有效期"由 ValidTime 与平台共同决定，不能只在本地配。
//
// # 为什么不在这里做限流
//
// 发码限流是站点策略（要看 site.login 的配额），属于调用方；本包只负责
// "把一条短信发出去"和"问一句这个码对不对"。
package sms

import (
	"context"
	"fmt"

	"myblogx/conf"
	"myblogx/internal/apperr"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dypns "github.com/alibabacloud-go/dypnsapi-20170525/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

// Kind 是短信验证码的使用场景。
//
// 只有三种，而不是照抄邮件那四个：手机号通道里"注册"与"登录"是同一条路径
// （查到就登录、查不到就建档），发码时根本无法区分二者。
type Kind int8

const (
	// KindAuth 是登录/注册共用的一次验证（手机号首次登录即建档）。
	KindAuth Kind = 1
	// KindPasswordProof 是"设置首个密码"前的复验，防止仅凭一个被盗的会话
	// 就把临时凭证升级成永久密码凭证。
	//
	// 取值刻意从 1 跳到 3：2 曾是"绑定手机号"场景，随该功能一起下线。
	// 号位不复用——历史日志里 OutId=bind_phone 的记录必须还能被正确解释。
	KindPasswordProof Kind = 3
)

// Sender 是短信能力的接缝。
//
// 抽成接口是为了让单测注入假实现，而不是每个用例都真的打阿里云。
type Sender interface {
	// Send 给 phone 发一条验证码；ttlMinutes 是平台侧的有效期，
	// intervalSecond 是平台侧对同一号码的最小重发间隔。
	//
	// 两个时间都由调用方按站点配置传进来：适配器自己写死常量的话，
	// 后台把窗口从 60 秒改成 120 秒，"我们这边放行、平台那边拒绝"就会互相打脸。
	// ctx 目前不参与 SDK 调用（该 SDK 是同步无 ctx 的），保留它是为了与
	// 本项目其它适配器（如 mail）保持同一签名，将来换 SDK 不必改调用方。
	Send(ctx context.Context, cfg conf.SMS, phone string, kind Kind, ttlMinutes, intervalSecond int) error
	// Check 核验 phone 与 code 是否匹配且在有效期内。
	//
	// 返回 (true, nil) 表示通过；(false, nil) 表示码错或已过期；
	// 只有"调用本身失败"才返回 error —— 调用方据此区分"用户的错"与"我们的错"，
	// 后者不该被当成"验证码错误"糊过去。
	Check(ctx context.Context, cfg conf.SMS, phone, code string) (bool, error)
}

// Client 是走阿里云 SDK 的实现。
type Client struct{}

// 编译期断言。
var _ Sender = (*Client)(nil)

// New 返回 SDK 实现。
func New() *Client { return &Client{} }

// newAPIClient 按当前配置构造 SDK 客户端。
//
// 每次现构造：配置来自运行时配置，后台改完密钥要立刻生效，不能只在启动时建一次。
// 构造本身不发网络请求，开销可以忽略。
func newAPIClient(cfg conf.SMS) (*dypns.Client, error) {
	return dypns.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		Endpoint:        tea.String(cfg.EndpointOrDefault()),
	})
}

// runtimeOptions 把配置里的超时换算成 SDK 的调用选项。
//
// 必须显式给超时：发码在登录/注册的同步路径上，SDK 默认超时会把一次请求
// 拖到分钟级（与邮件那边同一个理由，见 conf.Mail.Timeout）。
func runtimeOptions(cfg conf.SMS) *util.RuntimeOptions {
	ms := cfg.Timeout() * 1000
	return &util.RuntimeOptions{
		ReadTimeout:    tea.Int(ms),
		ConnectTimeout: tea.Int(ms),
	}
}

// Send 实现 Sender。
func (c *Client) Send(_ context.Context, cfg conf.SMS, phone string, kind Kind, ttlMinutes, intervalSecond int) error {
	if !cfg.Configured() {
		// 没配齐就是"发不出去"。不要静默成功：那会让用户拿着一个永远收不到的码干等。
		return apperr.New(apperr.CodeUserSmsSendFailed)
	}
	client, err := newAPIClient(cfg)
	if err != nil {
		return apperr.Wrap(apperr.CodeUserSmsSendFailed, err)
	}

	resp, err := client.SendSmsVerifyCodeWithOptions(&dypns.SendSmsVerifyCodeRequest{
		PhoneNumber:  tea.String(phone),
		SignName:     tea.String(cfg.SignName),
		TemplateCode: tea.String(cfg.TemplateCode),
		// 数字验证码，长度 6：手机号场景下 4 位太容易撞。
		CodeLength: tea.Int64(6),
		CodeType:   tea.Int64(1),
		// 同一号码在同一场景的有效期内重复发送时，让新码覆盖旧码。
		// 否则用户连点两次"发送"，两条码都能用，等于把爆破面翻倍。
		DuplicatePolicy: tea.Int64(1),
		// 平台侧对同一号码的最小重发间隔（秒）。取站点配置的窗口值：
		// 两边不一致时，用户会在我们放行之后被平台拒绝，错误信息还看不出来源。
		Interval:  tea.Int64(int64(max(intervalSecond, 1))),
		ValidTime: tea.Int64(int64(ttlMinutes)),
		// 不要平台把验证码回显在响应里。
		ReturnVerifyCode: tea.Bool(false),
		OutId:            tea.String(outID(kind)),
	}, runtimeOptions(cfg))
	if err != nil {
		return apperr.Wrap(apperr.CodeUserSmsSendFailed, err)
	}
	return assertSendOK(resp.Body)
}

// Check 实现 Sender。
//
// 这里所有 error 分支都用 sms_verify_failed 而不是 sms_send_failed：
// 用户看到的是"核验没跑通"，不是"短信没发出去"。
func (c *Client) Check(_ context.Context, cfg conf.SMS, phone, code string) (bool, error) {
	if !cfg.Configured() {
		return false, apperr.New(apperr.CodeUserSmsVerifyFailed)
	}
	client, err := newAPIClient(cfg)
	if err != nil {
		return false, apperr.Wrap(apperr.CodeUserSmsVerifyFailed, err)
	}

	resp, err := client.CheckSmsVerifyCodeWithOptions(&dypns.CheckSmsVerifyCodeRequest{
		PhoneNumber: tea.String(phone),
		VerifyCode:  tea.String(code),
	}, runtimeOptions(cfg))
	if err != nil {
		return false, apperr.Wrap(apperr.CodeUserSmsVerifyFailed, err)
	}
	body := resp.Body
	// 调用成功才谈得上"码对不对"：这里的失败是基础设施失败，不是用户的错。
	if body == nil || body.Code == nil || *body.Code != "OK" {
		return false, apperr.Wrap(apperr.CodeUserSmsVerifyFailed,
			fmt.Errorf("check sms verify code: code=%s", strOrEmpty(body)))
	}
	if body.Model == nil || body.Model.VerifyResult == nil {
		return false, nil
	}
	return *body.Model.VerifyResult == "PASS", nil
}

// assertSendOK 把 SDK 的"HTTP 200 但业务失败"翻译成错误。
//
// 阿里云 RPC 风格接口的失败可能落在 Code != "OK" 或 Success == false 上，
// 两者都要看：只看 HTTP 状态码会把"签名错误""模板未报备"当成发送成功，
// 而用户永远收不到短信。
func assertSendOK(body *dypns.SendSmsVerifyCodeResponseBody) error {
	if body == nil {
		return apperr.Wrap(apperr.CodeUserSmsSendFailed, fmt.Errorf("empty response body"))
	}
	code := ""
	if body.Code != nil {
		code = *body.Code
	}
	ok := body.Success != nil && *body.Success
	if code != "OK" || !ok {
		return apperr.Wrap(apperr.CodeUserSmsSendFailed,
			fmt.Errorf("send sms verify code failed: code=%s success=%v message=%s",
				code, ok, tea.StringValue(body.Message)))
	}
	return nil
}

// strOrEmpty 取核验响应的业务码，仅用于错误信息。
func strOrEmpty(body *dypns.CheckSmsVerifyCodeResponseBody) string {
	if body == nil || body.Code == nil {
		return ""
	}
	return *body.Code
}

// outID 给平台侧一个可读的业务来源标记，便于在控制台按场景筛记录。
func outID(kind Kind) string {
	switch kind {
	case KindAuth:
		return "auth"
	case KindPasswordProof:
		return "password_proof"
	default:
		return "unknown"
	}
}
