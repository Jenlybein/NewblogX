package conf

// Captcha 是第 12 个运行时配置段：图形验证码的「开关、入口闸门、图形与时效」参数。
//
// 为什么单开一段而不是塞进 `site`：
//   - `site` 段有匿名公开读（`GET /api/site/site`），图片尺寸、字符集、有效期这些
//     参数读者没有必要知道，混进去只是白白扩大公开面；
//   - 沿用 [ADR-0020](../../docs/adr/0020-config-section-registry-and-named-sections.md)
//     决策 3 的分法（`sms` 段只放发送凭证、登录策略留在 `site.login`）：
//     配置跟着"它服务的那个东西"走，而不是按"是不是策略"切。
//
// 「哪些入口要求验证码」也在本段（票 票 481 从 `site.login` 迁入）：它是图形验证码
// 这个机制自己的属性，混在 site 段里会让「验证码」页改不了自己最关键的五个开关。
// 读者站渲染表单需要知道这些布尔位，但它们**不再靠公开 site 段**得到 ——
// 公开只读端点 `GET /api/site/login_capabilities` 用 `captcha_required` 现读给出
// （已与总开关合取，见 internal/site/app 的 LoginCapabilities），
// 本段的渲染参数仍然只给后台读写。
//
// 本段的具名端点：`GET /site/admin/captcha` 与 `PUT /site/captcha`。
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
	// Captcha 是图形验证码的**总开关**（json 键名沿用历史写法 `captcha`）。
	//
	// 总开关关掉时四个入口一律不校验，这样"先整体关、再逐条开"的回滚路径是通的。
	Captcha bool `yaml:"captcha" json:"captcha"`
	// 下面三个是**入口闸门**：在总开关打开的前提下，逐个决定哪条路由真的校验。
	//
	// 默认全开（与总开关合起来等价于旧行为「一开就三条路由都校验」）。
	// 需要按入口分开的理由：登录和发码被滥用的方式不同 —— 发码要卡成本，
	// 登录要卡爆破；一刀切会让"只想给发码加验证码"做不到。
	CaptchaLogin       bool `yaml:"captcha_login" json:"captcha_login"`
	CaptchaEmailVerify bool `yaml:"captcha_email_verify" json:"captcha_email_verify"`
	CaptchaPhoneVerify bool `yaml:"captcha_phone_verify" json:"captcha_phone_verify"`
	// CaptchaRegister 是「注册专用入口」的闸门。
	//
	// 它只挂在**专门的注册路由**上（POST /users/email/register）。手机号注册没有
	// 独立路由 —— 它与登录共用 POST /users/phone（"首次验证即建档"），请求体层面
	// 分不出这次是注册还是登录，所以那条通道的验证码挂在**发码那一步**
	// （captcha_phone_verify）。这是设计约束不是遗漏，ADR-0033 决策 11 记了这件事。
	CaptchaRegister bool `yaml:"captcha_register" json:"captcha_register"`

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
	// 归一化会剔除重复字符与空白：旧实现的字符集里 `H` / `I` / `J` / `K` / `L`
	// 各有两处（重复了 5 个字符，40 个字符只剩 35 个可用），这种输入不再被接受。
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

	// FailMessage 是读者填错验证码时看到的那句话。
	//
	// 做成配置项而不是代码字面量：站点想把提示写得更像自己（"看不清？换一张再试"）
	// 不该发版。空串表示用默认文案（apperr 消息表里的那一句），这样没配过的站点
	// 行为与以前完全一致。
	FailMessage string `yaml:"fail_message" json:"fail_message"`
	// MaxSkew / DotCount 只对纯数字型生效：单字最大倾斜与背景圆点数量。
	// 两者的 0 都是**合法值**（不倾斜 / 不放点），不是"没填"；非纯数字驱动保留原值
	// 但不生效（清掉会让运营切到纯数字后看到与自己填的不一样的值）。
	MaxSkew  float64 `yaml:"max_skew" json:"max_skew" min:"0" max:"1"`
	DotCount int     `yaml:"dot_count" json:"dot_count" min:"0" max:"20"`
}
