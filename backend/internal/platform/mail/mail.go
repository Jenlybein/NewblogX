// Package mail 发送账号相关的验证码邮件。
//
// 底层是腾讯云邮件推送（SES）的 SendEmail 接口，**只走模板发信**：
// 正文由控制台审核过的模板渲染，代码只传 action / code / timeout 三个变量。
// 这样四个验证码场景共用同一个模板，不必为"注册/重置/绑定/登录"各建一个。
//
// 这是纯技术能力（调外部 API、拼参数），没有业务状态，所以放 platform
// 而不是 service。调用方只传配置、收件人与场景。
package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"myblogx/conf"
	"myblogx/internal/apperr"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

// DefaultEndpoint 是腾讯云 SES 的默认接入点。
//
// 国际站账号要覆盖成 ses.intl.tencentcloudapi.com，所以做成了配置项。
const DefaultEndpoint = "ses.tencentcloudapi.com"

// DefaultRegion 是 Region 留空时的兜底值。
//
// 官方只支持 ap-guangzhou 与 ap-hongkong 两个地域；给个默认值是为了
// "填了密钥忘了选地域"时不至于直接报错，而不是为了猜测用户意图。
const DefaultRegion = "ap-guangzhou"

// Kind 是验证码邮件的使用场景。
//
// 取值与 auth 模块传递的 kind（1-4）逐一对齐，装配层可以直接强转，
// 不必再维护一张映射表 —— 那种表迟早会和某一侧的改动错位。
type Kind int8

// 四种验证码场景。
const (
	KindRegister      Kind = 1
	KindResetPassword Kind = 2
	KindBindEmail     Kind = 3
	KindLogin         Kind = 4
)

// action 返回该场景在邮件正文里的动作名。
//
// 文案只有动作名不同，所以合并进一个模板变量；改文案时也只改这里。
func (k Kind) action() (string, bool) {
	switch k {
	case KindRegister:
		return "账号注册", true
	case KindResetPassword:
		return "重置密码", true
	case KindBindEmail:
		return "绑定邮箱", true
	case KindLogin:
		return "邮箱登录", true
	default:
		return "", false
	}
}

// Sender 是发送能力的接缝。
//
// 抽成接口是为了让单测能注入失败实现，而不必碰包级可变变量
// （旧实现用 `var smtpSend`，并行测试时不安全）。
type Sender interface {
	// SendTemplate 用模板发一封信。
	SendTemplate(ctx context.Context, cfg conf.Mail, msg TemplateMessage) error
}

// TemplateMessage 是一封待发的模板邮件。
type TemplateMessage struct {
	// To 收件人地址
	To string
	// Subject 邮件主题（模板发信也必须显式给）
	Subject string
	// Vars 模板变量；模板里用 {{key}} 引用
	Vars map[string]string
}

// Service 发送验证码邮件。
type Service struct {
	sender Sender
}

// New 构造使用腾讯云 SES 的服务。
func New() *Service {
	return &Service{sender: sesSender{}}
}

// NewWithSender 用指定的发送实现构造，供单测注入。
func NewWithSender(s Sender) *Service {
	if s == nil {
		s = sesSender{}
	}
	return &Service{sender: s}
}

// SendCode 发送一封验证码邮件。
//
// cfg 由调用方每次现取（来自运行时配置），所以后台改完立刻生效，
// 不需要重启进程 —— 这也是本模块不接受"启动时快照"的原因。
func (s *Service) SendCode(ctx context.Context, cfg conf.Mail, to string, kind Kind, code string, timeout int) error {
	action, ok := kind.action()
	if !ok {
		return apperr.New(apperr.CodeUserMailSendFailed)
	}
	if err := validate(cfg, to); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 5
	}

	// 错误码在这里统一兜住：无论哪个 Sender 实现失败，调用方拿到的
	// 都是 user.mail_send_failed —— 它靠这个码决定给用户看什么文案。
	if err := s.sender.SendTemplate(ctx, cfg, TemplateMessage{
		To:      to,
		Subject: subjectOf(cfg, action),
		Vars: map[string]string{
			"action":  action,
			"code":    code,
			"timeout": strconv.Itoa(timeout),
		},
	}); err != nil {
		return apperr.Wrap(apperr.CodeUserMailSendFailed, err)
	}
	return nil
}

// subjectOf 拼邮件主题。
//
// SES 即使走模板发信也要求显式传 Subject；这里沿用旧实现的「【站点名】 动作」，
// 让用户在收件箱里一眼看出是哪一站、在做什么。
func subjectOf(cfg conf.Mail, action string) string {
	name := strings.TrimSpace(cfg.FromName)
	if name == "" {
		return action
	}
	return fmt.Sprintf("【%s】 %s", name, action)
}

// validate 检查发信必需项。
//
// 这些条件不满足时**必须**在这里拒绝：放过去的话，SES 会返回
// InvalidParameter 之类的错误，日志里看不出是"没配"还是"配错了"。
// 尤其是 TemplateID 为 0 —— 模板发信没有模板 ID 根本无法渲染。
func validate(cfg conf.Mail, to string) error {
	switch {
	case strings.TrimSpace(to) == "":
		return apperr.New(apperr.CodeUserMailSendFailed)
	case strings.TrimSpace(cfg.SecretID) == "" || strings.TrimSpace(cfg.SecretKey) == "":
		return apperr.New(apperr.CodeUserMailSendFailed)
	case strings.TrimSpace(cfg.FromAddress) == "":
		return apperr.New(apperr.CodeUserMailSendFailed)
	case cfg.TemplateID == 0:
		return apperr.New(apperr.CodeUserMailSendFailed)
	default:
		return nil
	}
}

// sesSender 用腾讯云 SES 的 SendEmail 接口发模板邮件。
type sesSender struct{}

// SendTemplate 实现 Sender。
func (sesSender) SendTemplate(ctx context.Context, cfg conf.Mail, msg TemplateMessage) error {
	data, err := json.Marshal(msg.Vars)
	if err != nil {
		return err
	}

	client, err := newSESClient(cfg)
	if err != nil {
		return err
	}

	req := ses.NewSendEmailRequest()
	// 别名 + 空格 + 尖括号地址是 SES 要求的格式；FromName 留空时只给地址。
	req.FromEmailAddress = common.StringPtr(fromAddressOf(cfg))
	req.Subject = common.StringPtr(msg.Subject)
	req.Destination = common.StringPtrs([]string{msg.To})
	req.Template = &ses.Template{
		TemplateID:   common.Uint64Ptr(cfg.TemplateID),
		TemplateData: common.StringPtr(string(data)),
	}
	// 1 = 触发类（验证码这类即时邮件）。不设的话会走营销通道，
	// 延迟明显变高，验证码就失去意义了。
	req.TriggerType = common.Uint64Ptr(1)

	if _, err := client.SendEmailWithContext(ctx, req); err != nil {
		// 原始错误直接往上抛：错误码由 SendCode 统一包一次。
		// 底层原因要保留完整，日志才能区分"密钥错""发信地址未验证""模板不存在"。
		return err
	}
	return nil
}

// fromAddressOf 拼发件人。
func fromAddressOf(cfg conf.Mail) string {
	addr := strings.TrimSpace(cfg.FromAddress)
	name := strings.TrimSpace(cfg.FromName)
	if name == "" {
		return addr
	}
	return name + " <" + addr + ">"
}

// newSESClient 构造 SES 客户端。
//
// 超时必须显式给：SDK 默认 60 秒，而这条链路挂在登录/注册的同步路径上，
// 一次外部故障会把用户请求拖到分钟级。超时值来自运行时配置，
// 后台可以调，改完立刻生效。
func newSESClient(cfg conf.Mail) (*ses.Client, error) {
	credential := common.NewCredential(
		strings.TrimSpace(cfg.SecretID),
		strings.TrimSpace(cfg.SecretKey),
	)

	prof := profile.NewClientProfile()
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	prof.HttpProfile.Endpoint = endpoint
	prof.HttpProfile.ReqTimeout = cfg.Timeout()

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = DefaultRegion
	}

	client, err := ses.NewClient(credential, region, prof)
	if err != nil {
		return nil, err
	}
	return client, nil
}
