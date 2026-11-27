package site

import "time"

// 网站设置
//
// 曾经有一个 `mode`（1 社区 / 2 博客）：它只有枚举声明、默认值与后台下拉框，
// 全仓没有一处读它，于是"切换站点定位"这个动作什么都不会发生。ADR-0015 决定
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
// 这一节只剩「登录这件事本身」的两组策略：密码登录开关与登录失败限流。
//
// 其它字段都按**通道**搬回了各自的段（票 481）：
//   - 邮箱登录/注册、验证码有效期、发码限流 → conf.Mail；
//   - 手机号登录、验证码有效期、发码限流 → conf.SMS；
//   - QQ 登录开关 → conf.QQ；
//   - 图形验证码总开关与四个入口闸门 → conf.Captcha。
//
// 为什么按通道拆：字段留在 site.login 有三个讲不通的地方。
//  1. site 段有**匿名可读**的公开端点（GET /api/site/site 直接返回整个 conf.Site），
//     发码限流这类只关内部策略的参数跟着一起公开；
//  2. 管理端是按段授权编辑的（「邮件」页只该改邮件段），字段在 site.login 时
//     同一张「邮件」页改的数字要写进 site 段，谁生效取决于最后保存的是哪一页；
//  3. 邮件、短信、第三方登录、验证码各自的通道属性（能不能用、多久过期、限流多严）
//     本该只有一处事实源，混在 site 段里就出现了"改邮件要动站点配置"。
//
// 读者站需要的「能用哪些登录方式 + 哪些入口要验证码」改由公开只读端点
// GET /api/site/login_capabilities 现读各段拼出来（它不含任何凭据）。
type Login struct {
	// UsernamePwdLogin 是用户名密码登录开关。
	UsernamePwdLogin bool `yaml:"username_pwd_login" json:"username_pwd_login"`

	// 登录失败限流：这三项留在本段，因为它们描述的是"登录尝试"这件事，
	// 与具体走哪条通道无关（密码、邮箱、手机号登录共用同一套失败计数）。
	LoginFailWindowMinute int   `yaml:"login_fail_window_minute" json:"login_fail_window_minute"` // 登录失败统计窗口，单位：分钟
	LoginFailUserMax      int64 `yaml:"login_fail_user_max" json:"login_fail_user_max"`           // 单账号窗口内最大失败次数
	LoginFailIPMax        int64 `yaml:"login_fail_ip_max" json:"login_fail_ip_max"`               // 单IP窗口内最大失败次数
}

// 「一篇文章最多可选几个标签」的默认值与合法区间。
//
// 这三个数字与 `internal/article/domain` 里的同名常量是**同一件事的两份声明**：
// 配置段不能 import 业务模块的 domain（会成环），所以各写一份，由
// `internal/platform/config` 的一致性测试盯着它们不许漂。
const (
	DefaultArticleMaxTags = 5
	MinArticleMaxTags     = 1
	MaxArticleMaxTags     = 20
)

type Article struct {
	SkipExamining bool `yaml:"skip_examining" json:"skip_examining"` // 免审核

	// 「发布与展示」三个开关（ADR-0015 决策 2）。
	// 它们只表达"允许什么"，判定点在 app 层与读过滤处，不靠前端隐藏；
	// 默认值只写在 site_default_settings.yaml（这一段是自由键值块，
	// 深度合并会把老库缺的键按默认值补齐，所以不需要数据迁移）。
	AllowNonAdminPublish bool `yaml:"allow_non_admin_publish" json:"allow_non_admin_publish"` // 允许非管理员发布文章
	AllowNonAdminDraft   bool `yaml:"allow_non_admin_draft" json:"allow_non_admin_draft"`     // 允许非管理员创建草稿
	ShowNonAdminArticles bool `yaml:"show_non_admin_articles" json:"show_non_admin_articles"` // 显示非管理员发布的文章

	// SubmitDailyMax 是「投稿配额」的每日上限：每个普通用户每自然日可提交多少次
	// （ADR-0021，spec #302 决策 19）。管理员豁免是代码里的固定规则，不在这里配。
	//
	// 计量单位是**提交动作**而不是"发布成功的文章"：被打回也算一次，因为每次提交
	// 都会真实产生一次审核任务；因系统不可用而失败的提交由用例层释放预占，不占额度。
	//
	// 缺省值由 site_default_settings.yaml 显式写成 15；越界 / 0 / 负数在
	// internal/platform/config 的生效值模块里回落到默认值（那段是自由键值块，
	// 深度合并会把老库缺的键按默认值补齐，所以加这个字段不需要数据迁移）。
	SubmitDailyMax int `yaml:"submit_daily_max" json:"submit_daily_max"`

	// MaxTagsPerArticle 是一篇文章最多可选的标签数（默认 5，后台可改）。
	//
	// 为什么要有上限：标签在读者站是浏览入口（标签云、按标签搜索），一篇文章挂
	// 二十个标签等于告诉读者"它什么都算"，反而搜不出来。默认 5 覆盖"语言 + 框架
	// + 领域 + 类型 + 场景"这种典型组合仍有余量。
	//
	// 0 或越界按默认上限处理（见 domain.NormalizeTagLimit）：把这一项填成 0
	// 不该让全站都存不了草稿 —— 归一化放在 domain 的那一处，不在 HTTP 层与用例层
	// 各抄一份默认值。
	//
	// 缺省值由 site_default_settings.yaml 显式写成 5（这一段是自由键值块，
	// 深度合并会把老库缺的键按默认值补齐，所以加这个字段不需要数据迁移）。
	MaxTagsPerArticle int `yaml:"max_tags_per_article" json:"max_tags_per_article"`

	// ScoreEnabled 是「文章质量评分」总开关（默认 true）。
	//
	// 关掉之后**三条产生路径全部停摆**（提交时同步评分、后台重评、发布判定补算），
	// 读者站与写稿页不再显示任何分数，评分接口统一返回 article.score_disabled。
	//
	// 它与闸门是**父子关系**：评分关了，闸门就没有判据，因此归一化会强制把
	// quality_gate.enabled 写成 false（见 internal/site/app 的 normalizeArticle）。
	// 这条联动写在保存侧而不是读取侧 —— 读取侧取 and 会在库里留下一个"开着却
	// 永远不生效"的值，排查时极难看出它是死的。
	//
	// 关掉评分的**真实代价**：审核队列的风险列由分数派生（moderation.Evaluate 只吃
	// ScoreFacts），评分一停，同步风险扫描不再产生结论、风险列只能显示「未评估」。
	// 这是有意接受的取舍，不是缺陷。
	//
	// 缺省值由 site_default_settings.yaml 显式写成 true。
	ScoreEnabled bool `yaml:"score_enabled" json:"score_enabled"`

	// AIComplianceEnabled 是「AI 合规判定」（第③层）开关（默认 true）。
	//
	// 关掉之后文章不再走模型判定：质量已过即可见。它跳过的是"等 AI 判定+等人复核"
	// 这一步，不是"判定失败也放行" —— 分支只出现在质量判定通过之后
	// （app/publish_judge.go），拿不到分数与质量不过两条路径照旧不放行。
	//
	// 与「文章免审核」（skip_examining）的关系：两者最终都走到同一条"没有机器判据
	// ⇒ 提交即可见"的分支，区别是免审核是**临时放行**（一次性跳过质量与 AI），
	// 这个开关是**常态配置**（长期不调模型）。免审核不改名，但它不绕过关键词处置。
	AIComplianceEnabled bool `yaml:"ai_compliance_enabled" json:"ai_compliance_enabled"`

	// AIFailureAction 是「AI 合规判定不可用 / 失败时文章落到哪里」（默认 manual）。
	//
	// 取值：manual（转人工，现状）| hold（保持待可见，次日重投）| draft（退回草稿）|
	// release（直接放行，落「未判定」并记警告日志）。
	//
	// **只有"这次调用失败"受它支配**：模型判 suspect、判低风险 violation 都是**结论**
	// 不是失败，仍按结论转人工；额度用完也固定走 hold（重试注定再被拒、还会白扣一次），
	// 同样不看这个值。三种语义分开的理由见 app/outbox_wiring.go 的失败分支注释。
	AIFailureAction string `yaml:"ai_failure_action" json:"ai_failure_action" enums:"manual,hold,draft,release"`

	// QualityGate 是「发布质量闸门」的站点配置（票 306 / ADR-0023）。
	//
	// 它只是形状与默认值的载体：闸门开关、及格线、灰带宽度与四个档界全部
	// 自由可配、**不做交叉校验**（spec 决策 11）——允许保存"及格线 50 而
	// 31–59 叫不合格文章"这类自相矛盾的组合，代价由作者侧照旧显示档名承担。
	//
	// 缺省值由 site_default_settings.yaml 显式给出；越界值在 QualityGate.Effective
	// 一处收敛，不在用例层与 adapter 各抄一份。
	QualityGate QualityGate `yaml:"quality_gate" json:"quality_gate"`
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

	// 发送限流（ADR-0018）：同一发件人对同一收件人的滑动窗口与日额度。
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
	// 并账是扫表的运维动作，秒级周期只会放大写放大（ADR-0016「已否决方案」）。
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

// AI 合规判定不可用 / 失败时的四种落点（conf.Article.AIFailureAction 的取值域）。
//
// 定义在 conf 包而不是某个用例包里：它是**配置**的取值域，写入侧（site/app 的校验）、
// 读取侧（app 的分流）与文档都要引用同一份常量。散在用例包里会让"改一个值忘一处"
// 变成静默的字符串比较失败。
const (
	// AIFailureManual 转人工（默认值）。
	AIFailureManual = "manual"
	// AIFailureHold 保持待可见，次日重投。
	AIFailureHold = "hold"
	// AIFailureDraft 退回草稿，作者可重投。
	AIFailureDraft = "draft"
	// AIFailureRelease 直接放行并记警告日志（临时兜底）。
	AIFailureRelease = "release"
)

// AIFailureActions 是四档的完整清单（后台下拉与保存期校验共用一份）。
var AIFailureActions = []string{AIFailureManual, AIFailureHold, AIFailureDraft, AIFailureRelease}

// QualityGate 是「发布质量闸门」的配置形状（票 306 / ADR-0023）。
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
	// maxQualityGateGrayBandWidth 是灰带宽度上限（见 ADR-0023「代价」）。
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
