// Package app 是站点模块的用例层。
package app

import (
	"context"
	"fmt"
	"strings"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/site/domain"
)

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}

// SEO 是站点 SEO 摘要（给前端做 SSR 用）。
//
// 这里曾经还有 SiteTitle / Logo：它们的来源是 site_info.title / logo，而那两个
// 字段随 #192 一起删除，失去来源的它们也一并删掉。站点标题回到读者站的品牌常量，
// favicon 仍由 Icon（project.icon）供给。
type SEO struct {
	ProjectTitle string
	Icon         string
	Keywords     string
	Description  string
}

// AIInfo 是站点公开的 AI 信息（不包含密钥等敏感项）。
type AIInfo struct {
	// Enable 是 AI 总闸（服务端能否调模型）。
	Enable bool
	// AssistantEnabled 是读者站右下角浮窗开关（与总闸解耦，2026-11-28）。
	AssistantEnabled bool
	Nickname         string
	Avatar           string
	Abstract         string
}

// LoginCapabilities 是读者站「能用哪些登录方式 + 哪些入口要图形验证码」的公开快照。
//
// 为什么需要它（票 481）：这九个布尔位原本搭着公开读的 site 段一起发给读者站，
// 而它们已经按通道分散在 site.login / mail / sms / oauth / captcha 五段里 ——
// 邮件与短信段带密钥，只能后台读。读者站要回答的其实只有两个问题：
// "这个登录入口画不画"与"这个表单要不要渲染验证码框"，所以由后端把这五段现读
// 一遍、只挑布尔位拼成一份不含任何凭据的形状。
type LoginCapabilities struct {
	LoginMethods    LoginMethodSwitches
	CaptchaRequired CaptchaGates
}

// LoginMethodSwitches 是四条登录通道的开关。
type LoginMethodSwitches struct {
	UsernamePwd bool
	Email       bool
	Phone       bool
	QQ          bool
}

// CaptchaGates 是四个入口的图形验证码闸门（**生效值**：已与总开关合取）。
type CaptchaGates struct {
	Login       bool
	EmailVerify bool
	PhoneVerify bool
	Register    bool
}

// Service 是站点服务。
type Service struct {
	banners domain.BannerRepository
	images  domain.ImageResolver
	runtime domain.RuntimeConfig
	version string
	qqURL   func() string
	logger  Logger
	aiReady func() bool

	// contentFilterChanged 在词表段保存成功后被调用，用来按新词表重建过滤引擎。
	//
	// 做成回调而不是直接依赖引擎：平台包不认识过滤引擎的装配，#396 才把
	// 「过滤器」接到请求路径上。装配层调 SetContentFilterChangedHook 挂上来之前
	// 它为空，保存只刷新配置缓存（那也是"立即生效"的一半）。
	contentFilterChanged func(conf.ContentFilter)
}

// New 构造站点服务。
//
// aiReady 是「AI 现在能不能真的用」的现读判定（总闸开着且至少有一条启用的模型
// 配置）。它只用于保存前的校验：开「发布质量闸门」这个动作本身要求 AI 可用，
// 否则新站点上线第一天就是"所有投稿都被拒"（票 306 / ADR-0023）。
// 传 nil 表示不校验（只有测试会这样），生产装配必须给真实现。
func New(
	banners domain.BannerRepository,
	images domain.ImageResolver,
	runtime domain.RuntimeConfig,
	version string,
	qqURL func() string,
	logger Logger,
	aiReady func() bool,
) *Service {
	return &Service{
		banners: banners,
		images:  images,
		runtime: runtime,
		version: version,
		qqURL:   qqURL,
		logger:  logger,
		aiReady: aiReady,
	}
}

// QQLoginURL 返回 QQ 登录跳转地址。
//
// 每次现算：AppID 与回调地址都可能在后台被改，缓存住会让跳转指向旧地址。
func (s *Service) QQLoginURL() string {
	if s.qqURL == nil {
		return ""
	}
	return s.qqURL()
}

// ── 公开配置 ────────────────────────────────────────────────────

// PublicSite 返回公开的站点配置，并把构建版本号塞进 About.Version。
func (s *Service) PublicSite() conf.Site {
	site := s.runtime.Site()
	site.About.Version = s.version
	return site
}

// PublicSEO 返回 SEO 摘要。
func (s *Service) PublicSEO() SEO {
	site := s.runtime.Site()
	return SEO{
		ProjectTitle: site.Project.Title,
		Icon:         site.Project.Icon,
		Keywords:     site.Seo.Keywords,
		Description:  site.Seo.Description,
	}
}

// PublicAI 返回站点 AI 信息。
func (s *Service) PublicAI() AIInfo {
	ai := s.runtime.AI()
	return AIInfo{
		Enable:           ai.Enable,
		AssistantEnabled: ai.AssistantEnabled,
		Nickname:         ai.Nickname,
		Avatar:           ai.Avatar,
		Abstract:         ai.Abstract,
	}
}

// LoginCapabilities 返回登录方式与验证码闸门的公开快照（每次现读，不缓存）。
//
// 五个段各读一次：后台保存后下一次请求就该是新值，缓存住会让"刚关掉邮箱登录、
// 读者站还在显示入口"这种状态持续到重启。
func (s *Service) LoginCapabilities() LoginCapabilities {
	login := s.runtime.Site().Login
	mailConf := s.runtime.Mail()
	smsConf := s.runtime.SMS()
	oauthConf := s.runtime.OAuth()
	captchaConf := s.runtime.Captcha()

	// 闸门按总开关合取，而不是把总开关单独发给前端。
	// 读者站唯一要回答的问题是"这个表单画不画验证码框"；给出两个开关等于让每个
	// 调用点自己再与一次，漏一处就会渲染出一个永远不被校验的输入框
	// （用户填了不报错、看起来像验证码失效）。
	gates := CaptchaGates{}
	if captchaConf.Captcha {
		gates = CaptchaGates{
			Login:       captchaConf.CaptchaLogin,
			EmailVerify: captchaConf.CaptchaEmailVerify,
			PhoneVerify: captchaConf.CaptchaPhoneVerify,
			Register:    captchaConf.CaptchaRegister,
		}
	}

	return LoginCapabilities{
		LoginMethods: LoginMethodSwitches{
			UsernamePwd: login.UsernamePwdLogin,
			Email:       mailConf.EmailLogin,
			Phone:       smsConf.PhoneLogin,
			QQ:          oauthConf.QQLogin,
		},
		CaptchaRequired: gates,
	}
}

// ── 管理端配置 ──────────────────────────────────────────────────

// AdminSite 返回管理端的站点配置。
func (s *Service) AdminSite() conf.Site { return s.PublicSite() }

// AdminAI 返回管理端的 AI 配置。
//
// 这一段只剩站点级设置（总闸、展示信息、输入限制）：模型密钥与接口地址已经
// 搬到 ai_provider_models 表，由 AI 模块自己的接口管理并做遮蔽。
// 所以这里不再需要"遮蔽 + 占位符回滚"那一套。
func (s *Service) AdminAI() conf.AI { return s.runtime.AI() }

// UpdateSite 覆盖站点配置。
//
// 「开质量闸门」这个动作本身要求 AI 可用（票 306 / ADR-0023）：闸门是失败关闭的，
// 而 AI 默认是关闭的，两者叠在一起就是"新站点上线第一天所有投稿都被拒"。
// 因此把失败面从运行期挪到配置期——保存不了，这个状态就不存在。
func (s *Service) UpdateSite(site conf.Site) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	if err := normalizeArticle(&site.Article, s.aiReady); err != nil {
		return err
	}
	return s.runtime.UpdateSite(site)
}

// normalizeArticle 归一化并校验 site.article 段。
//
// 三条规则都写在**保存侧**而不是读取侧，理由见各自的注释：读取侧取 and 会在库里
// 留下一个「开着却永远不生效」的值，排查时极难看出它是死的。
//
//  1. 标签上限越界否决保存（见 validateArticleMaxTags 的长注释）；
//  2. 评分关 ⇒ 闸门强写 false（父子关系：没有评分就没有判据）；
//  3. ai_failure_action 只认四档，空串按 manual（老库缺键的零值）。
//
// 「开闸门要求 AI 可用」放在归一化**之后**：评分关掉时闸门已被写 false，此时
// 不该因为 AI 关着就拒绝保存一整段站点配置 —— 那次失败没有意义，而且会让
// 「关评分」这个动作在 AI 也关着的站点上根本存不进去。
func normalizeArticle(article *siteconf.Article, aiReady func() bool) error {
	if err := validateArticleMaxTags(article.MaxTagsPerArticle); err != nil {
		return err
	}

	action, err := normalizeAIFailureAction(article.AIFailureAction)
	if err != nil {
		return err
	}
	article.AIFailureAction = action

	// 评分关 ⇒ 闸门关。不报错、也不静默保留用户填的闸门值：它只是失去意义的
	// 从属项，写 false 才是诚实的表达。重新打开评分后闸门仍为 false（需手动再开），
	// 避免「关一下评分再打开，闸门却自己复活」这种意外副作用。
	if !article.ScoreEnabled {
		article.QualityGate.Enabled = false
	}

	// 闸门开启要求 AI 可用（票 306 / ADR-0023）：闸门是失败关闭的，而 AI 默认关闭，
	// 两者叠在一起就是「新站点上线第一天所有投稿都被拒」。把失败面挪到配置期。
	if article.QualityGate.Enabled && aiReady != nil && !aiReady() {
		return apperr.New(apperr.CodeArticleQualityGateRequiresAI)
	}
	return nil
}

// AI 合规判定不可用 / 失败时的四种落点由 conf/site 定义（配置的取值域只有一个来源），
// 这里用别名暴露给本包的调用方与测试，避免它们去记字符串字面量。
const (
	AIFailureManual  = siteconf.AIFailureManual
	AIFailureHold    = siteconf.AIFailureHold
	AIFailureDraft   = siteconf.AIFailureDraft
	AIFailureRelease = siteconf.AIFailureRelease
)

// AIFailureActions 是四档落点的完整清单（后台下拉与校验共用一份）。
var AIFailureActions = siteconf.AIFailureActions

// normalizeAIFailureAction 校验失败落点：空串按 manual，其余只认四档。
//
// 空串不报错的理由与处置档位一致：老库那段 JSON 里没有这个键，把它当非法值
// 会让「打开设置页什么都没改就点保存」直接失败。
func normalizeAIFailureAction(action string) (string, error) {
	switch trimmed := strings.TrimSpace(action); trimmed {
	case "":
		return AIFailureManual, nil
	case AIFailureManual, AIFailureHold, AIFailureDraft, AIFailureRelease:
		return trimmed, nil
	default:
		return "", apperr.NewWithMessage(apperr.CodeSiteArticleInvalid,
			"AI 合规判定的失败落点不合法：只能是 manual（转人工）/ hold（等次日重投）/ draft（退回草稿）/ release（直接放行），实际是 "+trimmed)
	}
}

// validateArticleMaxTags 校验"一篇文章最多可选几个标签"。
//
// 越界**不静默回落**（与 UpdateAI 的每日额度同一条口径）：后台是唯一写入口，
// 悄悄把 25 改成 5 只会让管理员以为改成功了。更要紧的是它有两个读面 ——
// 后台表单与读者站写稿页都按`GET /api/site/site` 里的值显示上限，
// 而真正拦人的后端按 `domain.NormalizeTagLimit` 回落默认值；两边不一致时
// 作者会选到"界面上合法、提交却被拒"的数量，那是最难解释的一种失败。
func validateArticleMaxTags(limit int) error {
	// 0 = 这一项从没配过（老库缺键解码出来的零值），按默认上限生效。
	// 放行它而不是拒绝，是因为整段保存会被一起否决 —— 运营改一次备案号
	// 不该因为一个从没碰过的字段失败。真正的默认值由 `domain.NormalizeTagLimit` 给。
	if limit == 0 {
		return nil
	}
	if limit >= siteconf.MinArticleMaxTags && limit <= siteconf.MaxArticleMaxTags {
		return nil
	}
	return apperr.NewWithMessage(
		apperr.CodeSiteArticleInvalid,
		fmt.Sprintf("一篇文章的标签上限必须在 %d–%d 之间，收到 %d",
			siteconf.MinArticleMaxTags, siteconf.MaxArticleMaxTags, limit),
	)
}

// UpdateAI 覆盖站点级 AI 设置。
//
// 每日上限在保存时校验（ADR-0028）：0 是合法值（不限），负数与超过上界的值
// 一律拒绝。这里的拒绝是**否决保存**而不是静默回落 —— 后台是唯一写入口，
// 悄悄改掉管理员填的数字只会让人以为改成功了。
func (s *Service) UpdateAI(ai conf.AI) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	if err := validateAIDailyLimits(ai.DailyLimits); err != nil {
		return err
	}
	return s.runtime.UpdateAI(ai)
}

// validateAIDailyLimits 校验七个场景的每日上限。
func validateAIDailyLimits(limits conf.DailyLimits) error {
	fields := []struct {
		name  string
		value int
	}{
		{"scoring", limits.Scoring},
		{"metainfo", limits.Metainfo},
		{"overwrite", limits.Overwrite},
		{"diagnose", limits.Diagnose},
		{"search_rewrite", limits.SearchRewrite},
		{"search_answer", limits.SearchAnswer},
		{"moderation", limits.Moderation},
	}
	for _, f := range fields {
		if f.value < 0 || f.value > config.MaxAIDailyLimit {
			return apperr.Wrap(apperr.CodeSiteAIDailyLimitInvalid,
				fmt.Errorf("daily_limits.%s=%d", f.name, f.value))
		}
	}
	return nil
}

// AdminMail 返回管理端的邮件配置，密钥用占位符遮蔽。
func (s *Service) AdminMail() conf.Mail {
	mail := s.runtime.Mail()
	mail.SecretKey = domain.SensitivePlaceholder
	return mail
}

// UpdateMail 覆盖邮件配置。
//
// 密钥的三种语义（占位符 = 没改 / 空串 = 真清除 / 其它 = 覆盖）由生效值模块的
// ResolveSensitive 统一判定，不在这里再写一份分支。
func (s *Service) UpdateMail(mail conf.Mail) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	mail.SecretKey = config.ResolveSensitive(mail.SecretKey, s.runtime.Mail().SecretKey)
	return s.runtime.UpdateMail(mail)
}

// AdminStorage 返回管理端的对象存储配置，密钥用占位符遮蔽。
func (s *Service) AdminStorage() conf.QiNiu {
	storage := s.runtime.Storage()
	storage.SecretKey = domain.SensitivePlaceholder
	return storage
}

// UpdateStorage 覆盖对象存储配置。
//
// 密钥占位符先收敛成旧值，再做字段校验：私有空间那条规则要看到真实密钥才成立。
func (s *Service) UpdateStorage(storage conf.QiNiu) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	storage.SecretKey = config.ResolveSensitive(storage.SecretKey, s.runtime.Storage().SecretKey)
	normalized, err := NormalizeStorage(storage)
	if err != nil {
		return err
	}
	return s.runtime.UpdateStorage(normalized)
}

// AdminOAuth 返回管理端的第三方登录配置，AppKey 用占位符遮蔽。
func (s *Service) AdminOAuth() conf.QQ {
	oauth := s.runtime.OAuth()
	oauth.AppKey = domain.SensitivePlaceholder
	return oauth
}

// UpdateOAuth 覆盖第三方登录配置。
func (s *Service) UpdateOAuth(oauth conf.QQ) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	oauth.AppKey = config.ResolveSensitive(oauth.AppKey, s.runtime.OAuth().AppKey)
	return s.runtime.UpdateOAuth(oauth)
}

// AdminSMS 返回管理端的短信通道配置，密钥用占位符遮蔽。
func (s *Service) AdminSMS() conf.SMS {
	sms := s.runtime.SMS()
	sms.AccessKeySecret = domain.SensitivePlaceholder
	return sms
}

// UpdateSMS 覆盖短信通道配置。
//
// 回显占位符原样提交时，用当前值覆盖回去——否则管理员只改模板号，
// 会把密钥改成一串星号（与邮件/存储的处理一致）。
func (s *Service) UpdateSMS(sms conf.SMS) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	sms.AccessKeySecret = config.ResolveSensitive(sms.AccessKeySecret, s.runtime.SMS().AccessKeySecret)
	return s.runtime.UpdateSMS(sms)
}

// AdminLog 返回管理端的日志配置。
func (s *Service) AdminLog() conf.Logrus { return s.runtime.Log() }

// UpdateLog 覆盖日志配置，但**只接受可以在后台改的那部分**。
//
// App / Dir / WALDir / StdoutFormat 是进程级配置：日志写到哪个目录、
// 用什么格式、服务名是什么，都由部署时的 settings.yaml 决定。
// 让后台改它们会出两种事故：
//   - 改了 Dir 而清理任务仍按旧目录算保留窗口，等于日志无人清理；
//   - 改了 Dir 之后容器里的路径不存在，写入直接失败，而失败日志也一起消失。
//
// 所以这几个字段一律用当前值覆盖回去，前端即使提交了也不生效。
func (s *Service) UpdateLog(log conf.Logrus) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	current := s.runtime.Log()
	log.App = current.App
	log.Dir = current.Dir
	log.WALDir = current.WALDir
	log.StdoutFormat = current.StdoutFormat
	return s.runtime.UpdateLog(log)
}

// PublicDeco 返回读者站要的装饰配置。
//
// 这段没有任何敏感字段，所以公开读与管理读返回同一份。
func (s *Service) PublicDeco() conf.Deco { return s.runtime.Deco() }

// AdminImageGC 返回管理端的孤儿图回收运行配置。
func (s *Service) AdminImageGC() conf.ImageGC { return s.runtime.ImageGC() }

// UpdateImageGC 覆盖孤儿图回收运行配置。
func (s *Service) UpdateImageGC(imageGC conf.ImageGC) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateImageGC(imageGC)
}

// AdminDeco 返回管理端的装饰配置。
func (s *Service) AdminDeco() conf.Deco { return s.PublicDeco() }

// AdminContentFilter 返回管理端的全站词表段（ADR-0032）。
//
// 这一段没有任何公开读：词表只给后台，读者站不会拿到它，匿名端点也不返回。
func (s *Service) AdminContentFilter() conf.ContentFilter { return s.runtime.ContentFilter() }

// SetContentFilterChangedHook 登记"词表保存之后"的回调。
//
// 过滤引擎（platform/contentfilter 的 Engine）在 #396 才接进请求路径；装配层把
// `engine.Rebuild(words, whitelist)` 挂到这里，保存即生效的那一半就闭环了。
// 传 nil 表示不关心（测试与尚未装配引擎的启动路径）。
func (s *Service) SetContentFilterChangedHook(fn func(conf.ContentFilter)) {
	s.contentFilterChanged = fn
}

// UpdateArticleReview 是「文章审核设置」两个子页的聚合写入口（2026-11-28）。
//
// 为什么需要它：审核规则页写 site.article 段，关键词命中处理页写 content_filter
// 段的 auto_dispose —— 一页两段、两页共用同一个端点。让前端各自去发两个既有端点
// 也可以，但那样「保存成功」这件事就被拆成两次网络往返，且中间失败会留下半套配置。
//
// 这里不做跨段事务（运行时配置服务是两段各自写的），但**先校验后写**：两段都先
// 归一化，任何一段不合法就一个字节都不写。「半保存」因此只可能发生在第二段写库时
// 存储层故障（几乎不可能，且日志会点名），而不是发生在最常见的「填错了值」上。
//
// contentFilter 为 nil 表示这次只改审核规则（关键词命中处理子页才传它）。
func (s *Service) UpdateArticleReview(site conf.Site, contentFilter *conf.ContentFilter) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}

	if err := normalizeArticle(&site.Article, s.aiReady); err != nil {
		return err
	}

	var normalizedFilter conf.ContentFilter
	if contentFilter != nil {
		normalized, err := normalizeContentFilter(*contentFilter)
		if err != nil {
			return err
		}
		normalizedFilter = normalized
	}

	// 两段都校验通过之后才开始写。顺序固定：先写 site（审核规则是主），
	// 再写 content_filter —— 如果第二段失败，第一段已落库但它是**合法值**，
	// 不会留下不可解释的状态，只是这次保存没完成（前端会收到错误并可重试）。
	if err := s.runtime.UpdateSite(site); err != nil {
		return err
	}
	if contentFilter == nil {
		return nil
	}
	if err := s.runtime.UpdateContentFilter(normalizedFilter); err != nil {
		return err
	}
	if s.contentFilterChanged != nil {
		s.contentFilterChanged(normalizedFilter)
	}
	return nil
}

// UpdateContentFilter 覆盖全站词表段。
//
// 保存前先在用例层把词条整理干净（去空白、去重、校验档位与类目）：词表是唯一写入口，
// 让坏数据写进库里只会把问题推迟到过滤引擎构建时 —— 那时就会变成启动失败。
// 写库并刷新缓存后立刻回调，让过滤器按新词表重建（原子切换，不阻塞在跑的请求）。
func (s *Service) UpdateContentFilter(contentFilter conf.ContentFilter) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	normalized, err := normalizeContentFilter(contentFilter)
	if err != nil {
		return err
	}
	if err := s.runtime.UpdateContentFilter(normalized); err != nil {
		return err
	}
	if s.contentFilterChanged != nil {
		s.contentFilterChanged(normalized)
	}
	return nil
}

// normalizeContentFilter 整理并校验词表段。
//
// 具体规则（去空白、丢空词条、按小写去重、校验档位/类目/单字词）在
// contentfilter.NormalizeWords —— 初始词表导入命令（cmd/cli 的 -filter-import）也走同一份，
// 导入能写进去的词，后台保存一定也能接受。
func normalizeContentFilter(contentFilter conf.ContentFilter) (conf.ContentFilter, error) {
	words := make([]contentfilter.Word, 0, len(contentFilter.Words))
	for _, entry := range contentFilter.Words {
		words = append(words, contentfilter.Word{
			Word:     entry.Word,
			Tier:     contentfilter.Tier(entry.Tier),
			Category: contentfilter.Category(entry.Category),
		})
	}
	normalized, err := contentfilter.NormalizeWords(words)
	if err != nil {
		// 错误文案由 contentfilter 给出（词表不合法的两个入口共用一份措辞）。
		return conf.ContentFilter{}, apperr.Wrap(apperr.CodeSiteContentFilterInvalid, err)
	}

	out := conf.ContentFilter{
		Words:            make([]conf.ContentFilterWord, 0, len(normalized)),
		Whitelist:        contentfilter.NormalizeWhitelist(contentFilter.Whitelist),
		WhitelistPhrases: contentfilter.NormalizeWhitelist(contentFilter.WhitelistPhrases),
		RetainDays:       normalizeRetainDays(contentFilter.RetainDays),
	}
	// 处置口径必须显式写进结果：out 是**逐字段手写的字面量**，漏一项就等于
	// 每次在后台保存词表都把它静默清零（"改一次词表，自动处置就悄悄变回默认"）。
	// 这类漏项不会报错、不会红测试，只会在某次保存之后才被发现，所以这里
	// 有专门的回归用例（TestUpdateContentFilterDefaultsAutoDisposeWhenBodyOmitsIt）。
	trimmedDispose, err := normalizeAutoDispose(contentFilter.AutoDispose)
	if err != nil {
		return conf.ContentFilter{}, err
	}
	out.AutoDispose = trimmedDispose
	for _, entry := range normalized {
		out.Words = append(out.Words, conf.ContentFilterWord{
			Word:     entry.Word,
			Tier:     string(entry.Tier),
			Category: string(entry.Category),
		})
	}
	return out, nil
}

// 关键词命中处理的三档取值（串行链路的第①环）。
//
// 值域从 mask/reject 扩成 release/draft/reject：
//   - release 就是改造前的 mask（打星照常往下走），旧值 mask 读成它；
//   - draft   命中落草稿（文章）/ 拒收（评论），配额不归还；
//   - reject  命中落已拒绝不可改（文章）/ 拒收（评论）。
//
// 为什么改名字而不是保留 mask：mask 描述的是「文本被怎么改写」，而这三个值描述
// 的是「命中之后文章去哪」—— 后者才是运营在这一步要决定的事。打星在 release
// 档里仍然发生，只是它不再是这一档的定义。
const (
	// ContentFilterActionRelease 只把命中替换成等长星号，文本照常入库并继续后续链路。
	ContentFilterActionRelease = "release"
	// ContentFilterActionDraft 命中落草稿（文章）/ 拒收（评论），配额不归还。
	ContentFilterActionDraft = "draft"
	// ContentFilterActionReject 命中落已拒绝且不可改（文章）/ 拒收（评论）。
	ContentFilterActionReject = "reject"
	// contentFilterActionLegacyMask 是改造前的取值，读到时等价于 release。
	contentFilterActionLegacyMask = "mask"
)

// normalizeAutoDispose 整理并校验按档位处置的口径。
//
// 空串 → mask 而不是报错：老库那段 JSON 与旧版后台请求体里都没有这两个字段，
// 把它们当非法值拒绝，会让"打开设置页什么都没改就点保存"直接失败；更要紧的是
// 前端还没跟上时，任何人保存词表都会把老配置变成一次 400。
//
// 非空且不在 mask/reject → 参数错误：写进库里只会让处置链路读到一条"读不懂的
// 口径"，到时候是按 mask 还是按 reject 处理都说不清。保存期就拒绝，运营当场能改。
func normalizeAutoDispose(in conf.ContentFilterAutoDispose) (conf.ContentFilterAutoDispose, error) {
	block, err := normalizeDisposeAction(in.BlockAction, "block_action", false)
	if err != nil {
		return conf.ContentFilterAutoDispose{}, err
	}
	// 观察词不给 reject：watch 是误报代价低的词（推广话术、口头语），拒发它们
	// 等于把「加微信」三个字写成违规内容。
	watch, err := normalizeDisposeAction(in.WatchAction, "watch_action", true)
	if err != nil {
		return conf.ContentFilterAutoDispose{}, err
	}
	return conf.ContentFilterAutoDispose{BlockAction: block, WatchAction: watch}, nil
}

// normalizeDisposeAction 校验单个处置取值：空串与旧值 mask 按 release，
// 其余只认 release / draft（观察档）或 release / draft / reject（拦截档）。
//
// field 只用于报错文案（运营要一眼看出是哪个档位填错了），不参与任何判定。
// watchOnly 为 true 时拒绝 reject —— 这是「观察档不设重罚」的落地点，
// 前端下拉已经不给它，但接口不能只靠前端。
func normalizeDisposeAction(action, field string, watchOnly bool) (string, error) {
	allowed := "release（打星照常）/ draft（落草稿）/ reject（拒绝）"
	if watchOnly {
		allowed = "release（打星照常）或 draft（落草稿）"
	}
	switch trimmed := strings.TrimSpace(action); trimmed {
	case "":
		return ContentFilterActionRelease, nil
	case contentFilterActionLegacyMask:
		// 旧库与旧请求体给的是 mask：等价于 release，就地升级而不是报错。
		return ContentFilterActionRelease, nil
	case ContentFilterActionRelease:
		return ContentFilterActionRelease, nil
	case ContentFilterActionDraft:
		return ContentFilterActionDraft, nil
	case ContentFilterActionReject:
		if watchOnly {
			return "", apperr.NewWithMessage(apperr.CodeSiteContentFilterInvalid,
				"命中处理口径不合法："+field+" 只能是 "+allowed+"，实际是 "+trimmed)
		}
		return ContentFilterActionReject, nil
	default:
		return "", apperr.NewWithMessage(apperr.CodeSiteContentFilterInvalid,
			"命中处理口径不合法："+field+" 只能是 "+allowed+"，实际是 "+trimmed)
	}
}

// DefaultContentFilterRetainDays 是命中记录保留天数的缺省值（与 ADR-0032 决策 10 一致）。
const DefaultContentFilterRetainDays = 90

// MaxContentFilterRetainDays 是保留天数的上限（十年）。
//
// 上限不是为了"数据量"（十年早就该归档了），而是为了让越界值有一个确定的落点：
// 0 / 负数当缺省（老配置没有这个键时就是 0），超上限钳到上限，都不报错 ——
// 与 deco 的 Normalize 同一口径（配置写歪不该让运营保存不了词表）。
const MaxContentFilterRetainDays = 3650

// normalizeRetainDays 把保留天数钳进 [1, MaxContentFilterRetainDays]。
//
// 0 表示"老配置里没有这个键"，按缺省值处理；负数与超上限钳到边界，不报错。
func normalizeRetainDays(days int) int {
	if days <= 0 {
		return DefaultContentFilterRetainDays
	}
	if days > MaxContentFilterRetainDays {
		return MaxContentFilterRetainDays
	}
	return days
}

// UpdateDeco 覆盖装饰配置。
//
// 写入前先 Normalize：越界钳到边界、缺省补默认值、非法值回落，**不报错**。
// 钳位放在用例层而不是 HTTP 层，是为了让"从哪条路径写进来"都受同一道防线
// 保护 —— 将来若多了导入配置的入口，也不会绕过它。
func (s *Service) UpdateDeco(deco conf.Deco) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateDeco(deco.Normalize())
}

// ── 轮播图 ──────────────────────────────────────────────────────

// BannerInput 是创建/更新轮播图的入参。
type BannerInput struct {
	CoverImageID ctype.ID
	Title        string
	Href         string
	Show         bool
}

// ListBanners 分页查轮播图，并回填封面图片 ID。
func (s *Service) ListBanners(ctx context.Context, q domain.BannerQuery) ([]domain.Banner, bool, error) {
	rows, hasMore, err := s.banners.List(ctx, q)
	if err != nil {
		return nil, false, err
	}
	s.attachCoverImageIDs(ctx, rows)
	return rows, hasMore, nil
}

// CreateBanner 新建轮播图。
func (s *Service) CreateBanner(ctx context.Context, in BannerInput) (domain.Banner, error) {
	cover, err := s.resolveCover(ctx, in.CoverImageID.Uint64())
	if err != nil {
		return domain.Banner{}, err
	}
	banner := domain.Banner{
		Show:         in.Show,
		Cover:        cover,
		CoverImageID: in.CoverImageID,
		Href:         in.Href,
		Title:        domain.NormalizeTitle(in.Title),
	}
	if err := s.banners.Create(ctx, &banner); err != nil {
		return domain.Banner{}, err
	}
	return banner, nil
}

// UpdateBanner 更新轮播图。
//
// 先解析封面再查存在性，与旧实现顺序一致：封面 ID 非法时优先报图片错误。
// 封面在上传前已按 16:9 定形，更新只有"整体替换"一种语义。
func (s *Service) UpdateBanner(ctx context.Context, id uint64, in BannerInput) error {
	cover, err := s.resolveCover(ctx, in.CoverImageID.Uint64())
	if err != nil {
		return err
	}
	if _, err := s.banners.Find(ctx, id); err != nil {
		return err
	}
	return s.banners.Update(ctx, id, domain.NormalizeTitle(in.Title), cover, in.Href, in.Show)
}

// RemoveBanners 批量删除轮播图。
func (s *Service) RemoveBanners(ctx context.Context, ids []uint64) (int, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	return s.banners.Delete(ctx, ids)
}

// resolveCover 由图片 ID 取封面地址。
func (s *Service) resolveCover(ctx context.Context, imageID uint64) (string, error) {
	if imageID == 0 {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	if s.images == nil {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	return s.images.URLByID(ctx, imageID)
}

// attachCoverImageIDs 批量回填封面图片 ID。
//
// 反查失败不影响列表本身：前端只会拿不到编辑回填的 ID。
func (s *Service) attachCoverImageIDs(ctx context.Context, rows []domain.Banner) {
	if len(rows) == 0 || s.images == nil {
		return
	}
	urls := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.Cover == "" {
			continue
		}
		if _, ok := seen[row.Cover]; ok {
			continue
		}
		seen[row.Cover] = struct{}{}
		urls = append(urls, row.Cover)
	}
	if len(urls) == 0 {
		return
	}
	idByURL, err := s.images.IDsByURLs(ctx, urls)
	if err != nil {
		s.logError("反查轮播图封面图片 ID 失败: %v", err)
		return
	}
	for i := range rows {
		if id, ok := idByURL[rows[i].Cover]; ok {
			rows[i].CoverImageID = ctype.NewID(id)
		}
	}
}

// normalizeIDs 去零去重。
func normalizeIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// logError 写一条错误日志。
func (s *Service) logError(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Errorf(format, args...)
}
