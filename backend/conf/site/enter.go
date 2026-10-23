package site

import "myblogx/models/enum"

// 网站设置
type SiteInfo struct {
	Title string        `yaml:"title" json:"title"`
	Logo  string        `yaml:"logo" json:"logo"`
	Beian string        `yaml:"beian" json:"beian"`
	Mode  enum.SiteMode `yaml:"mode" json:"mode" oneof:"1 2"` // 1 社区模式 2 博客模式
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
}

type Comment struct {
	SkipExamining bool `yaml:"skip_examining" json:"skip_examining"` // 免审核
}
