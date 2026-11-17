package conf

// Captcha 是第 12 个运行时配置段：图形验证码的「图形与时效」参数。
//
// 为什么单开一段而不是塞进 `site`：
//   - `site` 段有匿名公开读（`GET /api/site/site`），图片尺寸、字符集、有效期这些
//     参数读者没有必要知道，混进去只是白白扩大公开面；
//   - `site.login` 承载的是**登录策略**（哪种方式开放、发码限流多严），
//     而这里是**渲染细节**。沿用 [ADR-0020](../../docs/adr/0020-config-section-registry-and-named-sections.md)
//     决策 3 的分法（`sms` 段只放发送凭证、登录策略留在 `site.login`）：
//     策略留 `site.login`，渲染参数进本段。
//
// 「哪些入口要求验证码」**不在这里**：它是登录策略，见 `site.Login` 的三个闸门
// （`captcha_login` / `captcha_email_verify` / `captcha_phone_verify`），
// 那三个布尔位必须公开可读 —— 读者站要知道每个表单渲染不渲染验证码框。
//
// 本段只有后台具名端点：`GET /site/admin/captcha` 与 `PUT /site/captcha`。
//
// 字段适用性（不适用项会被忽略，后台按驱动禁用对应输入框，见
// internal/site/app/captcha.go 的 normalize）：
//
//	field              string  math  digit
//	length               ✓      —     ✓
//	width / height       ✓      ✓     ✓
//	source               ✓      —     —
//	noise_count          ✓      ✓     —
//	show_*_line          ✓      ✓     —
//	bg_color             ✓      ✓     —
//	max_skew / dot_count —      —     ✓
//	expire_seconds       ✓      ✓     ✓
type Captcha struct {
	// Driver 是图形验证码的类型：
	//   - string：字符型（字母 + 数字），旧实现的形态；
	//   - math：算术型（两位数四则运算），对脚本更有效；
	//   - digit：纯数字，方便手机键盘输入。
	Driver string `yaml:"driver" json:"driver" enums:"string,math,digit"`

	// Length 是验证码字符/运算数的位数：字符型指字符个数（2–8），
	// 纯数字指数字个数（3–8）。算术型不使用此项（题目固定为两位数运算）。
	Length int `yaml:"length" json:"length" min:"2" max:"8"`

	// Width / Height 是图片像素尺寸。前端按 base64 直接渲染，改这两项不需要发版。
	Width  int `yaml:"width" json:"width" min:"80" max:"400"`
	Height int `yaml:"height" json:"height" min:"30" max:"120"`

	// ExpireSeconds 是验证码有效期（秒），范围 60–3600。
	//
	// 它同时决定存储层的 TTL：验证码存在 Redis 里（多副本与重启都不会失效），
	// 每次生成按**当时的配置值**写 TTL，所以改它不需要重启，只影响新生成的图片。
	ExpireSeconds int `yaml:"expire_seconds" json:"expire_seconds" min:"60" max:"3600"`

	// Source 是字符型的字符集（仅在 driver=string 时使用）。
	// 归一化会剔除重复字符与空白：旧实现的字符集里 `IJKLHIJKL` 有重复字母，
	// 等于白送一半难度，这种输入不再被接受。
	Source string `yaml:"source" json:"source"`

	// NoiseCount 是噪点数量（字符型 / 算术型）：0–10，0 表示不放噪点。
	NoiseCount int `yaml:"noise_count" json:"noise_count" min:"0" max:"10"`

	// 三种干扰线各自一个开关，而不是暴露 base64Captcha 的位掩码常量：
	// 位掩码是库的内部编码（2/4/8），让运营去记位值是没道理的。
	ShowHollowLine bool `yaml:"show_hollow_line" json:"show_hollow_line"`
	ShowSlimeLine  bool `yaml:"show_slime_line" json:"show_slime_line"`
	ShowSineLine   bool `yaml:"show_sine_line" json:"show_sine_line"`

	// BgColor 是背景色，`#RRGGBB` 或 `#RRGGBBAA`（字符型 / 算术型）。
	// 默认值带 10/255 的不透明度，与旧实现一致。
	BgColor string `yaml:"bg_color" json:"bg_color"`

	// MaxSkew / DotCount 只对纯数字型生效：单字最大倾斜与背景圆点数量。
	MaxSkew  float64 `yaml:"max_skew" json:"max_skew" min:"0" max:"1"`
	DotCount int     `yaml:"dot_count" json:"dot_count" min:"0" max:"20"`
}
