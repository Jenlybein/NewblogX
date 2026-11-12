// 短信通道配置（阿里云号码认证服务 PNVS 的「短信认证」）。
//
// # 为什么单独一段
//
// 两个理由，都不是"习惯"：
//  1. AccessKeySecret 是机密，必须经 platform/config 的 sealForStore 加密落库；
//     而 site 段有一个**匿名可读**的公开端点（GET /site/site 直接返回整个
//     conf.Site），把它塞进 site 等于把密钥发给所有访客。
//  2. 这一段只描述"怎么把一条短信发出去"，不描述"什么时候允许发"。
//     开关、验证码有效期、发码限流都在 site.login 里，与邮件那几项并排
//     —— 同一个概念只有一处，避免两处配置互相打脸。
//
// # 为什么是 PNVS 而不是标准短信（dysmsapi）
//
// PNVS 短信认证免申请签名与模板，个人主体可用；代价是模板文案由平台提供、
// 只能填「验证码 / 有效期」两个变量。需要自定义文案时再换 dysmsapi，
// 届时只改 platform/sms 这一个适配器，配置形状可以保持不变。
package conf

// SMS 是短信通道配置。
//
// JSON tag 与 yaml tag 保持一致：既从 site_default_settings.yaml 读默认值，
// 也作为后台接口的请求/响应体（密钥字段回显时遮蔽）。
type SMS struct {
	// AccessKeyID 阿里云 AccessKey ID
	AccessKeyID string `yaml:"access_key_id" json:"access_key_id"`
	// AccessKeySecret 阿里云 AccessKey Secret（敏感，后台回显时用占位符遮蔽）
	AccessKeySecret string `yaml:"access_key_secret" json:"access_key_secret"`
	// Endpoint 接入点；留空按 DefaultSMSEndpoint
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	// SignName 短信签名。PNVS 短信认证用控制台赠送的签名，必须与赠送模板配套
	SignName string `yaml:"sign_name" json:"sign_name"`
	// TemplateCode 短信模板 Code。同样是控制台赠送的那一个
	TemplateCode string `yaml:"template_code" json:"template_code"`
	// TimeoutSec 单次调用超时（秒）；<=0 按 DefaultSMSTimeoutSec 处理。
	//
	// 和邮件同理：发码在登录/注册的**同步**路径上，没有上限的话一次上游卡顿
	// 会把请求挂到分钟级。
	TimeoutSec int `yaml:"timeout_sec" json:"timeout_sec"`
}

// DefaultSMSEndpoint 是 PNVS 的默认接入点。
const DefaultSMSEndpoint = "dypnsapi.aliyuncs.com"

// DefaultSMSTimeoutSec 是单次发码调用的默认超时秒数。
const DefaultSMSTimeoutSec = 10

// EndpointOrDefault 返回实际可用的接入点。
func (s SMS) EndpointOrDefault() string {
	if s.Endpoint == "" {
		return DefaultSMSEndpoint
	}
	return s.Endpoint
}

// Timeout 返回实际可用的超时秒数。
func (s SMS) Timeout() int {
	if s.TimeoutSec <= 0 {
		return DefaultSMSTimeoutSec
	}
	return s.TimeoutSec
}

// Configured 判断这段配置是否填齐（装配层据此决定是否注入发送器）。
//
// 只判断"能不能发"：开关不在这里，那是 site.login.phone_login 的事。
func (s SMS) Configured() bool {
	return s.AccessKeyID != "" && s.AccessKeySecret != "" &&
		s.SignName != "" && s.TemplateCode != ""
}
