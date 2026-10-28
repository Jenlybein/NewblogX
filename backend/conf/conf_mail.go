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

// Mail 是邮件发送配置。
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
