package site

// 网站设置
//
// 曾经有一个 `mode`（1 社区 / 2 博客）：它只有枚举声明、默认值与后台下拉框，
// 全仓没有一处读它，于是"切换站点定位"这个动作什么都不会发生。ADR-0017 决定
// 删除，站点表达差异改由「发布与展示」三个布尔开关（见 Article）承担。
type SiteInfo struct {
	Title string `yaml:"title" json:"title"`
	Logo  string `yaml:"logo" json:"logo"`
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

// 组件设置
type ComponentInfo struct {
	Title  string `yaml:"title" json:"title"`
	Enable bool   `yaml:"enable" json:"enable"`
}

// 首页右屏组件展示
type IndexRight struct {
	List []ComponentInfo `yaml:"list" json:"list"`
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
