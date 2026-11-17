package imagedisplay

import (
	"strings"

	"myblogx/conf"
)

// ConfigFrom 把运行时配置里的对象存储段翻译成派生配置。
//
// 为什么放在这个包而不是装配层：它是"配置字段 → 派生语义"的映射，
// 属于本包的领域知识，放这里才能被单测直接钉住；装配层只负责取配置。
func ConfigFrom(storage conf.QiNiu) Config {
	return Config{
		MediaBase:  normalizeBase(storage.Uri),
		Prefix:     strings.Trim(strings.TrimSpace(storage.Prefix), "/"),
		Delimiter:  storage.StyleDelimiter,
		Styles: map[Purpose]string{
			PurposeCover:   storage.ImageStyleCover,
			PurposeAvatar:  storage.ImageStyleAvatar,
			PurposeContent: storage.ImageStyleContent,
			PurposeChat:    storage.ImageStyleChat,
		},
		PrivateRead: storage.PrivateRead,
		// 闸门要两个开关同时成立：审核总开关关掉时，"过没过审"这件事本身不存在；
		// 闸门开关单独关掉是给审核链路故障留的紧急放行手段。
		GateEnabled: storage.AuditEnabled && storage.AuditGateEnabled,
	}
}

// normalizeBase 把访问域名收敛成"含协议头、无尾斜杠"。
//
// 与七牛适配层同一口径（那边还会强制 https）：这里不强制升级协议，
// 因为联调期可能指向只支持 HTTP 的测试域名，地址要按配置原样发出去。
func normalizeBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	if strings.Contains(base, "://") {
		return base
	}
	if strings.HasPrefix(base, "//") {
		return "https:" + base
	}
	return "https://" + base
}
