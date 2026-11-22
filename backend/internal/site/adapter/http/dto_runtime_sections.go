package http

import (
	"myblogx/conf"
)

// 运行时配置段的具名出入口（#283 / ADR-0020；第 11 段见 ADR-0032）。
//
// 每段一对具名形状：响应与请求体都直接以 conf 包的段结构为底层类型，
// 所以字段只有一份定义，契约（开放接口契约）里也不再出现
// `kernel.SiteConfigData = map[string]any` 这种随 name 变形的自由键值块。
// 两端前端各自从生成的 openapi.d.ts 派生类型，不再手写视图模型。
//
// 这里刻意不用 `type X = conf.Y`（真别名会被 swag 折回 conf 的名字），
// 而是定义新类型：契约里的 schema 名随段固定，且带 http adapter 包前缀。

type SiteSectionSiteResponse conf.Site
type SiteSectionSiteRequest conf.Site

type SiteSectionAiResponse conf.AI
type SiteSectionAiRequest conf.AI

type SiteSectionMailResponse conf.Mail
type SiteSectionMailRequest conf.Mail

type SiteSectionStorageResponse conf.QiNiu
type SiteSectionStorageRequest conf.QiNiu

type SiteSectionOauthResponse conf.QQ
type SiteSectionOauthRequest conf.QQ

type SiteSectionSmsResponse conf.SMS
type SiteSectionSmsRequest conf.SMS

type SiteSectionLogResponse conf.Logrus
type SiteSectionLogRequest conf.Logrus

type SiteSectionDecoResponse conf.Deco
type SiteSectionDecoRequest conf.Deco

type SiteSectionImageGcResponse conf.ImageGC
type SiteSectionImageGcRequest conf.ImageGC

// 第 11 段 content_filter（ADR-0032）：只有后台具名端点，没有公开读。
//
// 响应在词表之外多带两个字段：**必需类目**与**当前缺失的必需类目**。
//
// 为什么由后端下发而不是让后台自己数：这两个清单是过滤层的判据
// （contentfilter.RequiredCategories，启动与保存词表时打 Warn 用的也是它）。
// 前端自己再抄一份，就等于同一件事有了两个真相 —— 改一处忘另一处时，
// 红条会在一台机器上安静、在另一台上尖叫。
type SiteSectionContentFilterResponse struct {
	conf.ContentFilter
	// RequiredCategories 是"必须非空"的类目（当前是 politics 与 violence）。
	RequiredCategories []string `json:"required_categories"`
	// MissingRequiredCategories 是其中当前一条词都没有的（为空表示覆盖完整）。
	MissingRequiredCategories []string `json:"missing_required_categories"`
	// WhitelistStats 是白名单的三个口径（总数/词级/短语级/跨表重复）。
	//
	// 由后端算好下发，是因为"去重"是后端口径（`contentfilter.CountWhitelist`）：
	// 前端自己把两个数组长度相加，会出现"页面显示 50、体检显示 49"的偏差。
	WhitelistStats WhitelistStatsResponse `json:"whitelist_stats"`
}

// WhitelistStatsResponse 是白名单计数（去重后的口径）。
type WhitelistStatsResponse struct {
	Total      int `json:"total"`
	Words      int `json:"words"`
	Phrases    int `json:"phrases"`
	Duplicates int `json:"duplicates"`
}

type SiteSectionContentFilterRequest conf.ContentFilter

// 第 12 段 captcha（ADR-0033）：图形验证码的图形与时效参数，同样只有后台具名端点。
type SiteSectionCaptchaResponse conf.Captcha
type SiteSectionCaptchaRequest conf.Captcha
