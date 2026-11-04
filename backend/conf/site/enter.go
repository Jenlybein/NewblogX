package site

// 网站设置
//
// 曾经有一个 `mode`（1 社区 / 2 博客）：它只有枚举声明、默认值与后台下拉框，
// 全仓没有一处读它，于是"切换站点定位"这个动作什么都不会发生。ADR-0017 决定
// 删除，站点表达差异改由「发布与展示」三个布尔开关（见 Article）承担。
// 站点标题与 Logo 曾经也在这里（后台「基础信息」可改）：但读者站页头、浏览器
// 标题与站点品牌都另有单一来源，这两项改不改都不影响读者看到的东西，于是
// 整条链删除（#192）—— 品牌回到代码常量，favicon 仍走下面 Project.Icon。
type SiteInfo struct {
	Beian string `yaml:"beian" json:"beian"`
}

// 项目设置
type Project struct {
	Title   string `yaml:"title" json:"title"`
	Icon    string `yaml:"icon" json:"icon"`
	WebPath string `yaml:"web_path" json:"web_path"`
}

// 搜索引擎优化
type Seo struct {
	Keywords    string `yaml:"keywords" json:"keywords"`
	Description string `yaml:"description" json:"description"`
}

// 关于设置
type About struct {
	Version  string `yaml:"-" json:"version"`
	SiteDate string `yaml:"site_date" json:"site_date"`
	QQ       string `yaml:"qq" json:"qq"`
	Wechat   string `yaml:"wechat" json:"wechat"`
	Gitee    string `yaml:"gitee" json:"gitee"`
	BiliBili string `yaml:"bilibili" json:"bilibili"`
	Github   string `yaml:"github" json:"github"`
}

// 登录设置
type Login struct {
	QQLogin          bool `yaml:"qq_login" json:"qq_login"`
	UsernamePwdLogin bool `yaml:"username_pwd_login" json:"username_pwd_login"`
	EmailLogin       bool `yaml:"email_login" json:"email_login"`
	Captcha          bool `yaml:"captcha" json:"captcha"`
	EmailCodeTimeout int  `yaml:"email_code_timeout" json:"email_code_timeout"` // 邮箱验证码有效期，单位：分钟

	LoginFailWindowMinute int   `yaml:"login_fail_window_minute" json:"login_fail_window_minute"` // 登录失败统计窗口，单位：分钟
	LoginFailUserMax      int64 `yaml:"login_fail_user_max" json:"login_fail_user_max"`           // 单账号窗口内最大失败次数
	LoginFailIPMax        int64 `yaml:"login_fail_ip_max" json:"login_fail_ip_max"`               // 单IP窗口内最大失败次数

	EmailSendWindowSecond int   `yaml:"email_send_window_second" json:"email_send_window_second"` // 邮件发送限流窗口，单位：秒
	EmailSendPerEmailMax  int64 `yaml:"email_send_per_email_max" json:"email_send_per_email_max"` // 单邮箱窗口内最大发送次数
	EmailSendPerIPMax     int64 `yaml:"email_send_per_ip_max" json:"email_send_per_ip_max"`       // 单IP窗口内最大发送次数
}

type Article struct {
	SkipExamining bool `yaml:"skip_examining" json:"skip_examining"` // 免审核

	// 「发布与展示」三个开关（ADR-0017 决策 2）。
	// 它们只表达"允许什么"，判定点在 app 层与读过滤处，不靠前端隐藏；
	// 默认值只写在 site_default_settings.yaml（这一段是自由键值块，
	// 深度合并会把老库缺的键按默认值补齐，所以不需要数据迁移）。
	AllowNonAdminPublish bool `yaml:"allow_non_admin_publish" json:"allow_non_admin_publish"` // 允许非管理员发布文章
	AllowNonAdminDraft   bool `yaml:"allow_non_admin_draft" json:"allow_non_admin_draft"`     // 允许非管理员创建草稿
	ShowNonAdminArticles bool `yaml:"show_non_admin_articles" json:"show_non_admin_articles"` // 显示非管理员发布的文章
}

type Comment struct {
	SkipExamining bool `yaml:"skip_examining" json:"skip_examining"` // 免审核
}

// Chat 是私信的站点级策略（#202）。
//
// 陌生人私信开关放在这里，而不是某个用户配置行上的列：零 DDL 约束不允许
// 给既有表加列，而且这个策略本来就是站点统一的——关掉之后陌生人对谁都
// 发不出去，不再逐个用户判断。
//
// 默认值必须由 site_default_settings.yaml **显式**写成 false：
// 此结构体的零值恰好也是 false，若只靠"配置缺行"来得到关闭，
// 将来改默认值或删键都会静默改变语义，且没有测试会红。
type Chat struct {
	StrangerMsgEnabled bool `yaml:"stranger_msg_enabled" json:"stranger_msg_enabled"` // 允许陌生人发私信

	// 发送限流（ADR-0026）：同一发件人对同一收件人的滑动窗口与日额度。
	// 后台「站点 → 私信」卡可改，零 DDL —— 它们只是运行时配置的一个键，
	// 不给任何既有表加列。缺省值由 site_default_settings.yaml 显式写明。
	RateWindowSec int `yaml:"rate_window_sec" json:"rate_window_sec"` // 窗口长度（秒），窗口内允许 1 条
	RateDailyMax  int `yaml:"rate_daily_max" json:"rate_daily_max"`   // 自然日内允许的最大条数
}

// NormalizeChatRate 把非法 / 越界的限流参数收敛到合法区间。
//
// 后台填 0、负数或超大值时不能让限流静默失效：0 秒窗口等于不限流，
// 0 条日额度等于把所有私信都掐死。这里统一钳到 [1, 86400] 秒与 [1, 9999] 条，
// 缺省（0）回落到 ADR-0026 的默认 10 秒 / 99 条。
func NormalizeChatRate(windowSec, dailyMax int) (int, int) {
	const (
		defaultWindowSec = 10
		defaultDailyMax  = 99
		maxWindowSec     = 86400
		maxDailyMax      = 9999
	)
	if windowSec <= 0 || windowSec > maxWindowSec {
		windowSec = defaultWindowSec
	}
	if dailyMax <= 0 || dailyMax > maxDailyMax {
		dailyMax = defaultDailyMax
	}
	return windowSec, dailyMax
}
