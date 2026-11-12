package site

import "time"

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
//
// 这一节只放「登录策略」：哪种方式开放、验证码多久过期、发码限流多严。
// 短信通道的密钥与模板不在这里（那些是机密，要加密落库、且 site 段有
// 匿名可读的公开端点），见 conf.SMS 与运行时配置的 sms 段。
type Login struct {
	QQLogin          bool `yaml:"qq_login" json:"qq_login"`
	UsernamePwdLogin bool `yaml:"username_pwd_login" json:"username_pwd_login"`
	EmailLogin       bool `yaml:"email_login" json:"email_login"`
	// EmailRegister 是「邮箱注册」开关，与 EmailLogin 分开。
	//
	// 必须分开的理由：注册与登录是两件事，而绑邮箱要用发码接口。
	// 只用一个开关时，关掉邮箱注册会连"给已注册用户发绑定验证码"一起关掉。
	// 本项目的口径是「注册只走手机号」，所以它默认 false。
	EmailRegister bool `yaml:"email_register" json:"email_register"`
	// PhoneLogin 是「手机号 + 短信验证码」登录（兼注册）开关。
	PhoneLogin bool `yaml:"phone_login" json:"phone_login"`
	Captcha    bool `yaml:"captcha" json:"captcha"`
	EmailCodeTimeout int  `yaml:"email_code_timeout" json:"email_code_timeout"` // 邮箱验证码有效期，单位：分钟
	SmsCodeTimeout   int  `yaml:"sms_code_timeout" json:"sms_code_timeout"`     // 短信验证码有效期，单位：分钟

	LoginFailWindowMinute int   `yaml:"login_fail_window_minute" json:"login_fail_window_minute"` // 登录失败统计窗口，单位：分钟
	LoginFailUserMax      int64 `yaml:"login_fail_user_max" json:"login_fail_user_max"`           // 单账号窗口内最大失败次数
	LoginFailIPMax        int64 `yaml:"login_fail_ip_max" json:"login_fail_ip_max"`               // 单IP窗口内最大失败次数

	EmailSendWindowSecond int   `yaml:"email_send_window_second" json:"email_send_window_second"` // 邮件发送限流窗口，单位：秒
	EmailSendPerEmailMax  int64 `yaml:"email_send_per_email_max" json:"email_send_per_email_max"` // 单邮箱窗口内最大发送次数
	EmailSendPerIPMax     int64 `yaml:"email_send_per_ip_max" json:"email_send_per_ip_max"`       // 单IP窗口内最大发送次数

	// 短信发送限流：与邮件那三个并列，因为它们是同一类策略。
	// 分开计数（互不占用额度）：短信成本远高于邮件，且两条通道的滥用方式不同。
	SmsSendWindowSecond int   `yaml:"sms_send_window_second" json:"sms_send_window_second"` // 短信发送限流窗口，单位：秒
	SmsSendPerPhoneMax  int64 `yaml:"sms_send_per_phone_max" json:"sms_send_per_phone_max"` // 单手机号窗口内最大发送次数
	SmsSendPerIPMax     int64 `yaml:"sms_send_per_ip_max" json:"sms_send_per_ip_max"`       // 单IP窗口内最大发送次数
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

	// SubmitDailyMax 是「投稿配额」的每日上限：每个普通用户每自然日可提交多少次
	// （ADR-0030，spec #302 决策 19）。管理员豁免是代码里的固定规则，不在这里配。
	//
	// 计量单位是**提交动作**而不是"发布成功的文章"：被打回也算一次，因为每次提交
	// 都会真实产生一次审核任务；因系统不可用而失败的提交由用例层释放预占，不占额度。
	//
	// 缺省值由 site_default_settings.yaml 显式写成 15；越界 / 0 / 负数在
	// internal/platform/config 的生效值模块里回落到默认值（那段是自由键值块，
	// 深度合并会把老库缺的键按默认值补齐，所以加这个字段不需要数据迁移）。
	SubmitDailyMax int `yaml:"submit_daily_max" json:"submit_daily_max"`

	// Moderation 是机审前两层的高危词表（票 311 / spec #302 决策 22、27）。
	//
	// 词表是**运行期数据**（运营热改、不发版），"怎么判"的规则结构在
	// internal/article/domain/moderation 里。两档的差别：阻断级命中后将来可
	// 自动拒（本票只标红），观察级只标红、处置权交人。
	//
	// 缺省值只由 site_default_settings.yaml 显式给出（每个清单只有一个定义点）；
	// 这里不写第二份字面默认值。
	// QualityGate 是「发布质量闸门」的站点配置（票 306 / ADR-0032）。
	//
	// 它只是形状与默认值的载体：闸门开关、及格线、灰带宽度与四个档界全部
	// 自由可配、**不做交叉校验**（spec 决策 11）——允许保存"及格线 50 而
	// 31–59 叫不合格文章"这类自相矛盾的组合，代价由作者侧照旧显示档名承担。
	//
	// 缺省值由 site_default_settings.yaml 显式给出；越界值在 QualityGate.Effective
	// 一处收敛，不在用例层与 adapter 各抄一份。
	QualityGate QualityGate `yaml:"quality_gate" json:"quality_gate"`

	Moderation Moderation `yaml:"moderation" json:"moderation"`
}

// Moderation 是机审词表的运行期配置形状。
//
// 两个清单都整段替换（不做逐元素合并）：删掉一个词必须真的删掉，不能因为
// 默认值里还留着而"删了没反应"（合并语义见 internal/platform/config/merge.go）。
type Moderation struct {
	// BlockWords 阻断级词表：命中即高风险；后续票里允许自动拒（本票只标红）。
	BlockWords []string `yaml:"block_words" json:"block_words"`
	// WatchWords 观察级词表：命中只标红，处置权在人。
	WatchWords []string `yaml:"watch_words" json:"watch_words"`
}

// Comment 是评论的站点级策略（票 359）。
//
// 在票 359 之前这里只有一个"免审核"开关：打开就直发，等于评论完全裸奔。
// 现在补上两层判定，形状与文章那套**刻意保持一致** —— 词表同构、AI 结论同构，
// 运营只要理解一套规则。
type Comment struct {
	SkipExamining bool `yaml:"skip_examining" json:"skip_examining"` // 免审核（机器+AI 都通过时直发）

	// Moderation 是评论的机审词表（与文章同构）。
	Moderation Moderation `yaml:"moderation" json:"moderation"`

	// AI 是评论的 AI 合规判定（票 359）。
	AI CommentAI `yaml:"ai" json:"ai"`
}

// CommentAI 是评论 AI 判定的运行期参数。
//
// 单列一个结构而不是平铺进 Comment：这三个值是一组（开关 + 成本上限 + 超时），
// 平铺会让人以为它们彼此无关。
type CommentAI struct {
	// Enabled 打开后，评论在发布前会过一遍 AI 合规判定。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MaxTokens 是单次判定的输出上限，默认 1000。
	//
	// 结论只是一个短 JSON；给 1000 已经宽裕，钉住它能把每条评论的审核成本
	// 控制在可预期量级。
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
	// TimeoutSec 是单次判定的超时秒数，默认 8。
	//
	// 评论是**同步**路径：超时必须有上限，否则一次上游卡顿会把发评论的请求
	// 挂到分钟级。超时按"未判定"处理（留待人工），不按"通过"处理。
	TimeoutSec int `yaml:"timeout_sec" json:"timeout_sec"`
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

// Reconcile 是并账任务的运行期策略（#279）。
//
// 并账周期曾经是 cron 装配里的一行编译期常量（`counterSyncInterval = 30 * time.Minute`）：
// 想调就要改代码、重新发版。收成运行期配置之后，周期的**唯一来源**就是这一段，
// 代码里不再有第二份默认值。
//
// 它落在 site 段而不是新开一段：零 DDL 与既有运行期配置机制都要求"加配置不改表"，
// 而 site 段本来就是自由键值块（深度合并会把老库缺的键按默认值补齐）。
type Reconcile struct {
	// IntervalMinutes 是并账周期，单位分钟。
	//
	// 缺省（0）与负数回落到 30 分钟：0 周期会让调度器变成忙循环，而这更可能是
	// "配置写漏了"而不是"想每秒并一次"。上限钳到一天，下限 1 分钟 ——
	// 并账是扫表的运维动作，秒级周期只会放大写放大（ADR-0025「已否决方案」）。
	IntervalMinutes int `yaml:"interval_minutes" json:"interval_minutes"`
}

// 并账周期的默认值与边界（分钟）。
const (
	DefaultReconcileMinutes = 30
	maxReconcileMinutes     = 24 * 60
)

// Interval 返回生效的并账周期。
//
// 归一化放在这里而不是散在调度注册处：周期只有一个来源，
// 调度、用例、文档都从这一个换算拿值，任何人都不必再抄一份 30。
func (r Reconcile) Interval() time.Duration {
	minutes := r.IntervalMinutes
	if minutes <= 0 {
		minutes = DefaultReconcileMinutes
	}
	if minutes > maxReconcileMinutes {
		minutes = maxReconcileMinutes
	}
	return time.Duration(minutes) * time.Minute
}

// Chat 的默认值与合法区间不在这里声明（#284）：本包只声明配置形状，
// 归一化与默认 10 秒 / 99 条在 internal/platform/config 的生效值模块里，
// 避免同一个数字被复制成多份。数据库补种用的字面量仍由
// site_default_settings.yaml 显式给出（它是"库里没有这段时"的唯一来源）。

// QualityGate 是「发布质量闸门」的配置形状（票 306 / ADR-0032）。
type QualityGate struct {
	// Enabled 是闸门开关。零值 = 关闭，提交路径与引入闸门之前完全一致。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// PassScore 是标称及格线（0–100）。
	//
	// **实际放行线 = PassScore + GrayBandWidth**：灰带做在及格线**上方**，
	// 而不是上下两侧各留一半。理由是闸门的目的是让"同一篇文章今天能发、
	// 明天不能发"不发生——只有把放行线整体抬出抖动量程（±2 分）才做得到。
	PassScore int `yaml:"pass_score" json:"pass_score"`
	// GrayBandWidth 是灰带宽度（默认 2）：及格线上再让出这么多分才放行。
	GrayBandWidth int `yaml:"gray_band_width" json:"gray_band_width"`
	// 四个档界是五档位的上界：较差 / 不合格 / 常规 / 优质 / 精品。
	BandPoorMax        int `yaml:"band_poor_max" json:"band_poor_max"`
	BandUnqualifiedMax int `yaml:"band_unqualified_max" json:"band_unqualified_max"`
	BandNormalMax      int `yaml:"band_normal_max" json:"band_normal_max"`
	BandGoodMax        int `yaml:"band_good_max" json:"band_good_max"`
}

// 闸门默认值与合法边界。
const (
	// DefaultQualityGatePassScore 是默认及格线。
	DefaultQualityGatePassScore = 50
	// DefaultQualityGateGrayBandWidth 是默认灰带宽度。
	DefaultQualityGateGrayBandWidth = 2
	// DefaultQualityGateBandPoorMax 等四个是默认档界。
	DefaultQualityGateBandPoorMax        = 30
	DefaultQualityGateBandUnqualifiedMax = 59
	DefaultQualityGateBandNormalMax      = 80
	DefaultQualityGateBandGoodMax        = 89

	// minQualityGateScore / maxQualityGateScore 是分数字段的合法区间。
	minQualityGateScore = 0
	maxQualityGateScore = 100
	// maxQualityGateGrayBandWidth 是灰带宽度上限（见 ADR-0032「代价」）。
	maxQualityGateGrayBandWidth = 20
)

// Effective 把闸门配置收敛到合法区间；不做档界之间的交叉校验。
func (g QualityGate) Effective() QualityGate {
	g.PassScore = clampQualityScore(g.PassScore, DefaultQualityGatePassScore)
	g.GrayBandWidth = clampBand(g.GrayBandWidth)
	g.BandPoorMax = clampQualityScore(g.BandPoorMax, DefaultQualityGateBandPoorMax)
	g.BandUnqualifiedMax = clampQualityScore(g.BandUnqualifiedMax, DefaultQualityGateBandUnqualifiedMax)
	g.BandNormalMax = clampQualityScore(g.BandNormalMax, DefaultQualityGateBandNormalMax)
	g.BandGoodMax = clampQualityScore(g.BandGoodMax, DefaultQualityGateBandGoodMax)
	return g
}

func clampQualityScore(value, fallback int) int {
	if value < minQualityGateScore || value > maxQualityGateScore {
		return fallback
	}
	return value
}

func clampBand(value int) int {
	if value < 0 || value > maxQualityGateGrayBandWidth {
		return DefaultQualityGateGrayBandWidth
	}
	return value
}
