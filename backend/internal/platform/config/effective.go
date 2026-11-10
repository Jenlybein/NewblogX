package config

import (
	"myblogx/conf"
	siteconf "myblogx/conf/site"
)

// Chat 现读站点级私信策略（陌生人开关 + 限流参数）。
func (e *EffectiveConfig) Chat() siteconf.Chat {
	if e == nil || e.store == nil {
		return siteconf.Chat{}
	}
	return e.store.GetRuntimeChat()
}

// Storage 现读对象存储配置（失败关闭的消费者用它取当前域名）。
func (e *EffectiveConfig) Storage() conf.QiNiu {
	if e == nil || e.store == nil {
		return conf.QiNiu{}
	}
	return e.store.GetRuntimeStorage()
}

// 本包是「此刻生效值」的唯一模块（#284）：
//
//   - 默认值与合法范围：DefaultChatWindowSec / DefaultChatDailyMax / NormalizeChatRate；
//   - 补种与现读：RuntimeConfigService（registry.go 的九段登记 + runtime.go 的
//     LoadDefaults / seedMissing / GetRuntimeXxx + merge.go 的深度合并）；
//   - 敏感字段占位符：SensitivePlaceholder / ResolveSensitive；
//   - 段注册：RegisteredSections / LookupSection（#283 的唯一事实，这里只消费）。
//
// 消费点不再各自捕获 *RuntimeConfigService 再重复取反 / 归一化 / 判占位符，
// 只持 NewEffectiveConfig 给出的一个现读取值闭包：管理员在后台改完，下一次
// 读取就按新值判，不用重启。
//
// 「每天 99 条」只在本文件的 DefaultChatDailyMax 声明一次；apperr 的文案用
// 中性说法，限流器与 Redis 兜底都从这里派生，不再各抄一份 10 / 99。

// 私信发送限流的默认值与合法区间（ADR-0026）。
const (
	DefaultChatWindowSec = 10
	DefaultChatDailyMax  = 99

	minChatWindowSec = 1
	maxChatWindowSec = 86400
	minChatDailyMax  = 1
	maxChatDailyMax  = 9999
)

// ChatRate 是一对已经收敛到合法区间的生效值。
type ChatRate struct {
	WindowSec int
	DailyMax  int
}

// Pair 返回旧调用点要的 (窗口秒数, 日额度)。
func (r ChatRate) Pair() (int, int) { return r.WindowSec, r.DailyMax }

// NormalizeChatRate 把非法 / 越界的限流参数收敛到合法区间。
//
// 后台填 0、负数或超大值时不能让限流静默失效：0 秒窗口等于不限流，
// 0 条日额度等于把所有私信都掐死。缺省回落到本模块声明的默认值。
func NormalizeChatRate(windowSec, dailyMax int) ChatRate {
	if windowSec < minChatWindowSec || windowSec > maxChatWindowSec {
		windowSec = DefaultChatWindowSec
	}
	if dailyMax < minChatDailyMax || dailyMax > maxChatDailyMax {
		dailyMax = DefaultChatDailyMax
	}
	return ChatRate{WindowSec: windowSec, DailyMax: dailyMax}
}

// 投稿配额（每人每自然日可提交次数上限）的默认值与合法区间（ADR-0031，
// spec #302 决策 19）。
//
// 「每天 15 次」只在本文件声明一次：Redis 兜底（redis_article_quota 的 dailyMax<=0
// 分支）、生效值归一化与配置服务缺失时的回落都从这里派生，
// 与 chat 的「每天 99 条」同一条口径。数据库补种用的字面量仍由
// site_default_settings.yaml 显式给出。
const (
	// 投稿配额（每人每自然日可提交次数上限）的默认值与合法区间（ADR-0031，
	// spec #302 决策 19）。
	//
	// 「每天 15 次」只在本文件声明一次：Redis 兜底（redis_article_quota 的 dailyMax<=0
	// 分支）、生效值归一化与配置服务缺失时的回落都从这里派生，
	// 与 chat 的「每天 99 条」同一条口径。数据库补种用的字面量仍由
	// site_default_settings.yaml 显式给出。

	// DefaultArticleSubmitDailyMax 是默认的每日投稿上限。
	DefaultArticleSubmitDailyMax = 15

	minArticleSubmitDailyMax = 1
	maxArticleSubmitDailyMax = 9999
)

// AI 每日额度的默认值与合法区间（ADR-0038）。
//
// 「每个场景每天多少次」在这里声明一次：读取侧的越界回落、装配层缺省闭包、
// 单测里的最小装配都从这里派生。数据库补种用的字面量仍由
// site_default_settings.yaml 显式给出（与 chat / 投稿配额同一条口径）。
const (
	DefaultAIDailyLimitScoring       = 20
	DefaultAIDailyLimitMetainfo      = 50
	DefaultAIDailyLimitOverwrite     = 50
	DefaultAIDailyLimitDiagnose      = 20
	DefaultAIDailyLimitSearchRewrite = 50
	DefaultAIDailyLimitSearchAnswer  = 50
	DefaultAIDailyLimitModeration    = 20

	// MaxAIDailyLimit 是后台可填的上界；超过它按非法配置处理。
	MaxAIDailyLimit = 9999
)

// DefaultAIDailyLimits 返回一组内置默认额度。
func DefaultAIDailyLimits() conf.DailyLimits {
	return conf.DailyLimits{
		Scoring:       DefaultAIDailyLimitScoring,
		Metainfo:      DefaultAIDailyLimitMetainfo,
		Overwrite:     DefaultAIDailyLimitOverwrite,
		Diagnose:      DefaultAIDailyLimitDiagnose,
		SearchRewrite: DefaultAIDailyLimitSearchRewrite,
		SearchAnswer:  DefaultAIDailyLimitSearchAnswer,
		Moderation:    DefaultAIDailyLimitModeration,
	}
}

// NormalizeAIDailyLimits 把越界值收敛到合法区间。
//
// 0 是**合法值**（不限），不能像投稿配额那样把 0 也回落到默认 ——那会让
// "我不想限制这个场景"变成"给它 20 次"。
// 负数与超过上界的值更可能是手改库或旧数据造成的，一律回落到该场景的默认值；
// 正常写入路径在保存时就被拒绝（见 site 用例层的校验）。
func NormalizeAIDailyLimits(in conf.DailyLimits) conf.DailyLimits {
	def := DefaultAIDailyLimits()
	normalize := func(value, fallback int) int {
		if value < 0 || value > MaxAIDailyLimit {
			return fallback
		}
		return value
	}
	return conf.DailyLimits{
		Scoring:       normalize(in.Scoring, def.Scoring),
		Metainfo:      normalize(in.Metainfo, def.Metainfo),
		Overwrite:     normalize(in.Overwrite, def.Overwrite),
		Diagnose:      normalize(in.Diagnose, def.Diagnose),
		SearchRewrite: normalize(in.SearchRewrite, def.SearchRewrite),
		SearchAnswer:  normalize(in.SearchAnswer, def.SearchAnswer),
		Moderation:    normalize(in.Moderation, def.Moderation),
	}
}

// AIDailyLimits 现读七个场景的每日额度（已归一化）。
//
// 每次调用现读，所以后台改完下一次请求即生效（ADR-0038 的验收标准之一）。
func (e *EffectiveConfig) AIDailyLimits() conf.DailyLimits {
	if e == nil || e.store == nil {
		return DefaultAIDailyLimits()
	}
	return NormalizeAIDailyLimits(e.store.GetRuntimeAI().DailyLimits)
}

// AIDailyLimitsFunc 返回消费点唯一该持有的现读取值闭包。
func (e *EffectiveConfig) AIDailyLimitsFunc() func() conf.DailyLimits {
	if e == nil {
		return func() conf.DailyLimits { return DefaultAIDailyLimits() }
	}
	return e.AIDailyLimits
}

// NormalizeArticleSubmitDailyMax 把非法 / 越界的每日投稿上限收敛到合法区间。
//
// 0 与负数一律回落到默认值，而不是解释成"禁止投稿"：这两个值更可能是
// "配置写漏了"而不是产品意图，而按字面执行会让全站用户当天一篇文章都提交不了。
func NormalizeArticleSubmitDailyMax(dailyMax int) int {
	if dailyMax < minArticleSubmitDailyMax || dailyMax > maxArticleSubmitDailyMax {
		return DefaultArticleSubmitDailyMax
	}
	return dailyMax
}

// ArticleSubmitDailyMax 现读每日投稿上限的生效值。
//
// 每次请求现读（不缓存），所以后台改完**下一次请求**即按新值判 —— 票 304 的
// 验收标准之一。
func (e *EffectiveConfig) ArticleSubmitDailyMax() int {
	if e == nil || e.store == nil {
		return DefaultArticleSubmitDailyMax
	}
	return NormalizeArticleSubmitDailyMax(e.store.GetRuntimeArticle().SubmitDailyMax)
}

// ArticleSubmitDailyMaxFunc 返回消费点唯一该持有的现读取值闭包。
func (e *EffectiveConfig) ArticleSubmitDailyMaxFunc() func() int {
	if e == nil {
		return func() int { return DefaultArticleSubmitDailyMax }
	}
	return e.ArticleSubmitDailyMax
}

// SensitivePlaceholder 是管理员接口回显敏感字段时用的占位符。
//
// 前端保存时会把这个值原样传回来，服务端据此判断"用户没有改密钥"。
const SensitivePlaceholder = "******"

// ResolveSensitive 把后台回传的敏感字段收敛成真正要落库的值：
//
//   - 占位符 = 没改，沿用当前值（不把星号写回库）；
//   - 空串 = 真清除（管理员要能撤销一个已泄露的密钥）；
//   - 其它字符串 = 覆盖。
//
// 三种语义只在这里判定一次，管理端三个密钥字段不再各写一份分支。
func ResolveSensitive(incoming, current string) string {
	if incoming == SensitivePlaceholder {
		return current
	}
	return incoming
}

// EffectiveConfig 持有「此刻生效值」的读入口。
//
// 它不缓存任何值：每次调用都回到 RuntimeConfigService 的现读快照，
// 所以后台保存后不需要重建本对象、更不需要重启进程。
type EffectiveConfig struct {
	store *RuntimeConfigService
}

// NewEffectiveConfig 构造生效值入口。store 为 nil 时所有读取走安全默认值
// （展示开关按"开"处理，限流按模块默认值处理，失败关闭/宁可多出内容）。
func NewEffectiveConfig(store *RuntimeConfigService) *EffectiveConfig {
	return &EffectiveConfig{store: store}
}

// ChatRate 现读私信限流生效值，并收敛到合法区间。
func (e *EffectiveConfig) ChatRate() ChatRate {
	if e == nil || e.store == nil {
		return NormalizeChatRate(0, 0)
	}
	chat := e.store.GetRuntimeChat()
	return NormalizeChatRate(chat.RateWindowSec, chat.RateDailyMax)
}

// ChatRateFunc 返回消费点唯一该持有的现读取值闭包。
func (e *EffectiveConfig) ChatRateFunc() func() (int, int) {
	return func() (int, int) { return e.ChatRate().Pair() }
}

// OnlyAdminAuthors 现读「显示非管理员发布文章」开关的反面：
// 为真表示公开读面只出管理员作者的文章（ADR-0017 决策 4）。
//
// 取反只在这里做一次；article / search / 右栏 / AI 检索四处消费点都拿它。
func (e *EffectiveConfig) OnlyAdminAuthors() bool {
	if e == nil || e.store == nil {
		return false
	}
	return !e.store.GetRuntimeArticle().ShowNonAdminArticles
}

// OnlyAdminAuthorsFunc 返回展示开关的现读取值闭包。
func (e *EffectiveConfig) OnlyAdminAuthorsFunc() func() bool {
	if e == nil {
		return func() bool { return false }
	}
	return e.OnlyAdminAuthors
}

// Article 现读文章相关开关，供只关心 site.article 的消费点使用。
//
// 配置服务缺失时按业务默认（三个发布/展示开关全开、免审核关闭、
// 投稿配额按模块默认值）兜底：宁可多出内容，也不要因为装配缺失把全站作者锁死。
func (e *EffectiveConfig) Article() siteconf.Article {
	if e == nil || e.store == nil {
		return siteconf.Article{
			SkipExamining:        false,
			AllowNonAdminPublish: true,
			AllowNonAdminDraft:   true,
			ShowNonAdminArticles: true,
			SubmitDailyMax:       DefaultArticleSubmitDailyMax,
		}
	}
	return e.store.GetRuntimeArticle()
}

// SectionNames 返回九段登记项的名字（复用 #283 的 RegisteredSections 外部行为）。
func (e *EffectiveConfig) SectionNames() []SectionName {
	entries := RegisteredSections()
	out := make([]SectionName, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

// HasSection 查询段是否登记（漏登记在写库前就会失败，见 registry_test.go）。
func (e *EffectiveConfig) HasSection(name string) bool {
	_, ok := LookupSection(name)
	return ok
}
