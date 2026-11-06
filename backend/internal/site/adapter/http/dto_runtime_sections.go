package http

import (
	"myblogx/conf"
)

// 九段运行时配置的具名出入口（#283 / ADR-0029）。
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

type SiteSectionLogResponse conf.Logrus
type SiteSectionLogRequest conf.Logrus

type SiteSectionUploadResponse conf.Upload
type SiteSectionUploadRequest conf.Upload

type SiteSectionDecoResponse conf.Deco
type SiteSectionDecoRequest conf.Deco

type SiteSectionImageGcResponse conf.ImageGC
type SiteSectionImageGcRequest conf.ImageGC
