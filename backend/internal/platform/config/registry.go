package config

import (
	"myblogx/conf"
)

// SectionName 是运行时配置段在库里的 name，也是 HTTP 具名端点与契约 schema
// 共用的那一份事实。九段之外没有隐式段。
type SectionName string

const (
	SectionSite    = "site"
	SectionAI      = "ai"
	SectionMail    = "mail"
	SectionStorage = "storage"
	SectionOAuth   = "oauth"
	SectionSMS     = "sms"
	SectionLog     = "log"
	SectionUpload  = "upload"
	SectionDeco    = "deco"
	SectionImageGC = "image_gc"
)

// sectionEntry 是单段在注册表里的登记项。
type sectionEntry struct {
	Name SectionName
	// Defaults 取该段默认值（补种与空缓存回退）。
	Defaults func(conf.RuntimeDefaults) any
	// Decode 把 payloads 合并默认值后解码成该段的强类型值。
	Decode func(*RuntimeConfigService, map[string]string) any
	// Apply 把已写入的值同步进缓存；返回 false 表示类型不匹配。
	Apply func(*RuntimeConfig, any) bool
}

// sectionRegistry 是九段唯一事实：补种、全量解码、缓存应用、HTTP 具名出入口
// 全部从它派生。漏登记的段在写入路径直接失败，不再"警告后缓存未更新"。
var sectionRegistry = []sectionEntry{
	{
		Name:     SectionSite,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Site },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionSite, s.defaults.Site, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.Site)
			if ok {
				c.Site = typed
			}
			return ok
		},
	},
	{
		Name:     SectionAI,
		Defaults: func(d conf.RuntimeDefaults) any { return d.AI },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionAI, s.defaults.AI, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.AI)
			if ok {
				c.AI = typed
			}
			return ok
		},
	},
	{
		Name:     SectionMail,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Mail },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionMail, s.defaults.Mail, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.Mail)
			if ok {
				c.Mail = typed
			}
			return ok
		},
	},
	{
		Name:     SectionStorage,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Storage },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionStorage, s.defaults.Storage, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.QiNiu)
			if ok {
				c.Storage = typed
			}
			return ok
		},
	},
	{
		Name:     SectionOAuth,
		Defaults: func(d conf.RuntimeDefaults) any { return d.OAuth },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionOAuth, s.defaults.OAuth, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.QQ)
			if ok {
				c.OAuth = typed
			}
			return ok
		},
	},
	{
		Name:     SectionSMS,
		Defaults: func(d conf.RuntimeDefaults) any { return d.SMS },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionSMS, s.defaults.SMS, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.SMS)
			if ok {
				c.SMS = typed
			}
			return ok
		},
	},
	{
		Name:     SectionLog,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Log },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionLog, s.defaults.Log, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.Logrus)
			if ok {
				c.Log = typed
			}
			return ok
		},
	},
	{
		Name:     SectionUpload,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Upload },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionUpload, s.defaults.Upload, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.Upload)
			if ok {
				c.Upload = typed
			}
			return ok
		},
	},
	{
		Name:     SectionDeco,
		Defaults: func(d conf.RuntimeDefaults) any { return d.Deco },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionDeco, s.defaults.Deco, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.Deco)
			if ok {
				c.Deco = typed
			}
			return ok
		},
	},
	{
		Name:     SectionImageGC,
		Defaults: func(d conf.RuntimeDefaults) any { return d.ImageGC },
		Decode: func(s *RuntimeConfigService, p map[string]string) any {
			return decodeOrDefault(SectionImageGC, s.defaults.ImageGC, p, s.logger)
		},
		Apply: func(c *RuntimeConfig, v any) bool {
			typed, ok := v.(conf.ImageGC)
			if ok {
				c.ImageGC = typed
			}
			return ok
		},
	},
}

// RegisteredSections 返回全部登记项（顺序稳定、只读）。
func RegisteredSections() []sectionEntry { return sectionRegistry }

// LookupSection 按段名查登记项；未登记返回 false。
func LookupSection(name string) (sectionEntry, bool) {
	for _, entry := range sectionRegistry {
		if string(entry.Name) == name {
			return entry, true
		}
	}
	return sectionEntry{}, false
}
