// 邮件（腾讯云 SES）配置
//
// 取代旧的 conf.Email：底层从 SMTP 换成腾讯云邮件推送的 SendEmail 接口，
// 所以字段是按 SES 的口径设计的，不再有 smtp host/port/auth_code。
//
// 注意两个容易踩的限制（来自官方文档）：
//  1. SendEmail 默认仅支持模板发信 —— 所以必须有 TemplateID，
//     正文由控制台审核过的模板渲染，代码只传变量。
//  2. Region 只支持 ap-guangzhou 与 ap-hongkong。
package conf

// Mail 是邮件通道配置：前半段是"怎么把信发出去"（腾讯云 SES 的凭据与模板），
// 后半段是"这条通道允不允许用、验证码多久过期、发码限流多严"（票 票 481 从
// site.login 迁入）。两半放在同一段，是因为它们都只服务邮箱验证码这一条通道。
//
// JSON tag 与 yaml tag 保持一致：这份结构既要从 site_default_settings.yaml
// 读默认值，也要作为后台接口的请求/响应体。
type Mail struct {
	// SecretID 腾讯云 API 密钥 ID
	SecretID string `yaml:"secret_id" json:"secret_id"`
	// SecretKey 腾讯云 API 密钥 Key（敏感，后台回显时用占位符遮蔽）
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	// Region 地域；只支持 ap-guangzhou / ap-hongkong
	Region string `yaml:"region" json:"region"`
	// Endpoint 可选：国际站账号需要覆盖成 ses.intl.tencentcloudapi.com
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	// FromAddress 发信地址；必须已在控制台验证通过，否则报 NotAuthenticatedSender
	FromAddress string `yaml:"from_address" json:"from_address"`
	// FromName 发件人显示名
	FromName string `yaml:"from_name" json:"from_name"`
	// TemplateID 验证码模板 ID；模板正文见文档，变量为 action / code / timeout
	TemplateID uint64 `yaml:"template_id" json:"template_id"`
	// TimeoutSec 单次调用超时（秒）；<=0 按 DefaultMailTimeoutSec 处理
	TimeoutSec int `yaml:"timeout_sec" json:"timeout_sec"`

	// ── 邮箱验证码通道的策略（票 票 481 从 site.login 迁入）──
	//
	// 为什么跟着邮件段走，而不是留在 site.login：这几项描述的是**这条通道**的
	// 属性（开不开、验证码多久过期、发码限流多严），而 site 段有匿名可读的公开
	// 端点（GET /api/site/site 返回整个 conf.Site），发码限流跟着一起公开说不通。
	// 拆过来之后，后台「邮件」页改完只写这一段，不再与站点段抢同一个数字。
	//
	// 读者站要的只是"邮箱登录开没开"这一个布尔位，由公开只读端点
	// GET /api/site/login_capabilities 现读给出，不再公开整个 site 段。
	//
	// EmailLogin 是「邮箱验证码登录」开关；启用前需要先配好腾讯云 SES，
	// 否则用户点「获取验证码」会直接报错。
	EmailLogin bool `yaml:"email_login" json:"email_login"`
	// EmailRegister 是「邮箱注册」开关，与 EmailLogin 分开。
	//
	// 必须分开的理由：注册与登录是两件事，而绑邮箱要用发码接口。
	// 只用一个开关时，关掉邮箱注册会连"给已注册用户发绑定验证码"一起关掉。
	// 本项目的口径是「注册只走手机号」，所以它默认 false。
	EmailRegister bool `yaml:"email_register" json:"email_register"`
	// EmailCodeTimeout 是邮箱验证码有效期，单位：分钟。
	EmailCodeTimeout int `yaml:"email_code_timeout" json:"email_code_timeout"`
	// EmailSendWindowSecond 是邮件发送限流窗口，单位：秒。
	EmailSendWindowSecond int `yaml:"email_send_window_second" json:"email_send_window_second"`
	// EmailSendPerEmailMax 是单邮箱在限流窗口内允许的最大发送次数。
	EmailSendPerEmailMax int64 `yaml:"email_send_per_email_max" json:"email_send_per_email_max"`
	// EmailSendPerIPMax 是单 IP 在限流窗口内允许的最大发送次数。
	EmailSendPerIPMax int64 `yaml:"email_send_per_ip_max" json:"email_send_per_ip_max"`
}

// DefaultMailTimeoutSec 是发信调用的默认超时秒数。
//
// 必须显式给一个上限：SDK 默认 60 秒，在登录/注册的同步路径上会
// 把一次请求拖到分钟级，违反"任何外部调用都要有超时"的硬约束。
const DefaultMailTimeoutSec = 10

// Timeout 返回实际可用的超时秒数。
func (m Mail) Timeout() int {
	if m.TimeoutSec <= 0 {
		return DefaultMailTimeoutSec
	}
	return m.TimeoutSec
}
