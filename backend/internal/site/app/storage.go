package app

import (
	"fmt"
	"strings"

	"myblogx/conf"
	"myblogx/internal/apperr"
)

const (
	// MaxImageStyleNameLength 是样式名长度上限。
	//
	// 七牛并没有给出硬上限，但样式名会进 URL 路径。32 与对象存储里其他短标识
	// （bucket、region）保持同一量级，也足够表达 cover / avatar / thumb800 这类名字。
	MaxImageStyleNameLength = 32

	// MinDownloadExpirySeconds / MaxDownloadExpirySeconds 是下载签名有效期的边界。
	//
	// 下限 60：更短的签名对浏览器缓存毫无意义，只会让图片频繁 401。
	// 上限 7 天：再长就等于把私有空间的访问权长期外泄。
	MinDownloadExpirySeconds = 60
	MaxDownloadExpirySeconds = 7 * 24 * 3600

	// MinAuditTimeoutSeconds / MaxAuditTimeoutSeconds 是审核结论容忍时长。
	MinAuditTimeoutSeconds = 5
	MaxAuditTimeoutSeconds = 3600

	// defaultStyleDelimiter 是七牛控制台的默认样式分隔符。
	defaultStyleDelimiter = "-"
	// defaultDownloadExpirySeconds 与 defaultAuditTimeoutSeconds 是"没填"时的回落值。
	defaultDownloadExpirySeconds = 3600
	defaultAuditTimeoutSeconds   = 60
)

// allowedStyleDelimiters 是允许的样式分隔符。
//
// 只放行单字符、且不会改变 URL 路径结构的字符：
// `/` 会让样式后缀变成路径分隔（反解 object key 时容易与目录混淆），
// `?` `#` 会截断 URL —— 这些一个都不能放进来。
var allowedStyleDelimiters = map[string]struct{}{
	"-": {},
	"_": {},
	"!": {},
	"~": {},
	".": {},
}

// NormalizeStorage 整理并校验对象存储段（ADR-0034 新增的图片样式与审核字段）。
//
// 口径与验证码段一致：**越界即拒绝，不静默夹取**。运维把样式名填成
// `"my style"`（带空格）却"保存成功"，线上表现是全站图片 404，而七牛的
// 404 不会告诉你"样式名非法"，只会安静地找不到文件。
//
// 只有"没填（空串 / 0）"才回落到默认值（分隔符、有效期、超时），
// 这与运行时配置 merge 的语义一致。
func NormalizeStorage(input conf.QiNiu) (conf.QiNiu, error) {
	out := input

	styles := []struct {
		label string
		value *string
	}{
		{"封面样式", &out.ImageStyleCover},
		{"头像样式", &out.ImageStyleAvatar},
		{"正文样式", &out.ImageStyleContent},
		{"私信样式", &out.ImageStyleChat},
	}
	for _, style := range styles {
		normalized, err := normalizeStyleName(style.label, *style.value)
		if err != nil {
			return conf.QiNiu{}, err
		}
		*style.value = normalized
	}

	delimiter := strings.TrimSpace(input.StyleDelimiter)
	if delimiter == "" {
		delimiter = defaultStyleDelimiter
	}
	if _, ok := allowedStyleDelimiters[delimiter]; !ok {
		return conf.QiNiu{}, storageInvalid("样式分隔符只能是 %s 之一，收到 %q",
			strings.Join(styleDelimiterList(), " "), input.StyleDelimiter)
	}
	out.StyleDelimiter = delimiter

	out.DownloadExpiry = defaultIfZero(input.DownloadExpiry, defaultDownloadExpirySeconds)
	if out.DownloadExpiry < MinDownloadExpirySeconds || out.DownloadExpiry > MaxDownloadExpirySeconds {
		return conf.QiNiu{}, storageInvalid("下载签名有效期需在 %d–%d 秒之间，收到 %d",
			MinDownloadExpirySeconds, MaxDownloadExpirySeconds, out.DownloadExpiry)
	}

	out.AuditTimeoutSeconds = defaultIfZero(input.AuditTimeoutSeconds, defaultAuditTimeoutSeconds)
	if out.AuditTimeoutSeconds < MinAuditTimeoutSeconds || out.AuditTimeoutSeconds > MaxAuditTimeoutSeconds {
		return conf.QiNiu{}, storageInvalid("审核超时需在 %d–%d 秒之间，收到 %d",
			MinAuditTimeoutSeconds, MaxAuditTimeoutSeconds, out.AuditTimeoutSeconds)
	}

	// 私有空间必须能签名：没有密钥就只能签发一个空令牌，图片会全部 401。
	// 这一步在密钥占位符收敛之后调用，所以这里拿到的是真实密钥。
	if out.PrivateRead && (strings.TrimSpace(out.AccessKey) == "" || strings.TrimSpace(out.SecretKey) == "") {
		return conf.QiNiu{}, storageInvalid("开启「私有空间」必须同时填好 AccessKey 与 SecretKey，否则图片链接无法签名")
	}

	return out, nil
}

// normalizeStyleName 校验单个样式名。空串合法（表示该档用原图）。
func normalizeStyleName(label, value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", nil
	}
	if len(name) > MaxImageStyleNameLength {
		return "", storageInvalid("%s 名称最长 %d 个字符，收到 %d 个",
			label, MaxImageStyleNameLength, len(name))
	}
	for _, r := range name {
		isDigit := r >= '0' && r <= '9'
		isUpper := r >= 'A' && r <= 'Z'
		isLower := r >= 'a' && r <= 'z'
		isSafe := r == '-' || r == '_'
		if !isDigit && !isUpper && !isLower && !isSafe {
			return "", storageInvalid("%s 名称只能包含字母、数字、减号与下划线，收到 %q", label, name)
		}
	}
	return name, nil
}

// styleDelimiterList 返回允许的分隔符清单（用于错误文案，顺序固定便于测试）。
func styleDelimiterList() []string { return []string{"-", "_", "!", "~", "."} }

// storageInvalid 把具体原因包成对外的配置错误。
//
// 用 NewWithMessage 而不是 Wrap：这句"哪个字段越什么界"要能原样显示给运营，
// 而 Wrap 的具体原因会被错误码的通用文案盖掉（响应层只看 userMessage）。
func storageInvalid(format string, args ...any) error {
	return apperr.NewWithMessage(apperr.CodeSiteStorageInvalid, fmt.Sprintf(format, args...))
}
