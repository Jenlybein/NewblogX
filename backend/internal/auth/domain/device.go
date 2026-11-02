package domain

import (
	"regexp"
	"strings"

	ua "github.com/mileusna/useragent"
)

// DeviceType 是设备的粗分类。
//
// 只分四类：用户能凭"手机 / 平板 / 电脑 / 爬虫"这句话认出自己的设备，
// 再细的型号属于设备指纹范畴，本票不做。
type DeviceType string

// 设备类型常量。
const (
	DeviceDesktop DeviceType = "desktop"
	DeviceMobile  DeviceType = "mobile"
	DeviceTablet  DeviceType = "tablet"
	DeviceBot     DeviceType = "bot"
	DeviceUnknown DeviceType = "unknown"
)

// Device 是从 User-Agent 解析出的可读设备信息。
//
// 它不落库、不参与鉴权，只在"登录设备"列表里做展示与聚合的分组键。
// 解析结果必须稳定：同一 UA 多次解析得到同一结果，否则列表每次刷新会抖动。
type Device struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	Type           DeviceType
}

// browserAliases 把解析库的浏览器名收敛成产品里统一叫法。
//
// 白名单而不是"原样透传"：解析库对认不出的 UA 会把整段字符串当浏览器名
// （例如 "test-agent" 甚至中文），直接透传等于把 UA 原文又漏回页面。
var browserAliases = map[string]string{
	"chrome":          "Chrome",
	"chromium":        "Chromium",
	"edge":            "Edge",
	"firefox":         "Firefox",
	"safari":          "Safari",
	"opera":           "Opera",
	"vivaldi":         "Vivaldi",
	"brave":           "Brave",
	"samsung browser": "Samsung Internet",
}

var (
	reMicroMessenger = regexp.MustCompile(`MicroMessenger/([0-9.]+)`)
	reUCBrowser      = regexp.MustCompile(`UCBrowser/([0-9.]+)`)
	reQQClient       = regexp.MustCompile(`QQ/([0-9.]+)`)
)

// ParseUserAgent 解析 UA 字符串。空 UA、任意脏串都返回"未知设备"，不 panic。
//
// 纯函数：不读配置、不查库、不依赖时间，因此可以放心用在读路径上。
func ParseUserAgent(raw string) Device {
	if strings.TrimSpace(raw) == "" {
		return Device{Type: DeviceUnknown}
	}

	parsed := ua.Parse(raw)

	browser, version := browserFromRaw(raw)
	if browser == "" {
		if alias, ok := browserAliases[strings.ToLower(parsed.Name)]; ok {
			browser = alias
			version = parsed.Version
		}
	}

	osName, osVersion := normalizeOS(parsed.OS, parsed.OSVersion)

	return Device{
		Browser:        browser,
		BrowserVersion: majorVersion(version),
		OS:             osName,
		OSVersion:      osVersion,
		Type:           deviceTypeOf(parsed),
	}
}

// browserFromRaw 先认 UA 里"解析库容易认错"的国产客户端。
//
// 微信 Android、UC 的 UA 里都带着 Chrome 段，解析库会先报 Chrome；
// 对用户来说这是微信/UC，必须以原始串里的标记为准。
func browserFromRaw(raw string) (name, version string) {
	switch {
	case strings.Contains(raw, "MicroMessenger"):
		return "WeChat", firstGroup(reMicroMessenger, raw)
	case strings.Contains(raw, "UCBrowser"):
		return "UC Browser", firstGroup(reUCBrowser, raw)
	case strings.Contains(raw, "QQ/"):
		return "QQ", firstGroup(reQQClient, raw)
	default:
		return "", ""
	}
}

// normalizeOS 把系统名收敛成产品叫法，并只保留能帮用户认设备的主版本。
//
// Windows/macOS/Linux 的 UA 版本没有展示价值（Windows 11 在 UA 里也是 NT 10.0，
// macOS 长期停在 10.15），保留反而误导；iOS / Android 的主版本是有效信息。
func normalizeOS(name, version string) (osName, osVersion string) {
	switch strings.ToLower(name) {
	case "windows":
		return "Windows", ""
	case "macos":
		return "macOS", ""
	case "linux":
		return "Linux", ""
	case "chromeos":
		return "ChromeOS", ""
	case "ios":
		return "iOS", numericVersion(version)
	case "android":
		return "Android", numericVersion(version)
	default:
		return "", ""
	}
}

func deviceTypeOf(p ua.UserAgent) DeviceType {
	switch {
	case p.Bot:
		return DeviceBot
	case p.Tablet:
		return DeviceTablet
	case p.Mobile:
		return DeviceMobile
	case p.Desktop:
		return DeviceDesktop
	default:
		return DeviceUnknown
	}
}

// IsUnknown 判断有没有解析出任何可读信息。
func (d Device) IsUnknown() bool { return d.Browser == "" && d.OS == "" }

// Key 是聚合用的设备签名：只含浏览器 + 系统 + 类型，不含版本号。
//
// 不含版本号是刻意的：同一台设备浏览器小版本升级不该在列表里分裂成两条。
// 返回空串表示"没解析出可读信息"，由聚合方改用 UA 原文兜底。
func (d Device) Key() string {
	if d.IsUnknown() {
		return ""
	}
	return strings.ToLower(d.Browser + "|" + d.OS + "|" + string(d.Type))
}

// Label 是给用户看的人话描述，例如 "Chrome 131 · Windows"。
//
// 与 UA 原文的区别：这里只保留浏览器、系统（移动端再补类型），
// 不把 Mozilla/5.0、WebKit 版本这类噪音倒给用户。
func (d Device) Label() string {
	if d.IsUnknown() {
		return "未知设备"
	}

	browser := d.Browser
	if browser == "" {
		browser = "未知浏览器"
	}
	if d.BrowserVersion != "" {
		browser += " " + d.BrowserVersion
	}

	system := d.OS
	if system == "" {
		system = "未知系统"
	}
	if d.OSVersion != "" {
		system += " " + d.OSVersion
	}

	label := browser + " · " + system
	switch d.Type {
	case DeviceMobile:
		label += "（手机）"
	case DeviceTablet:
		label += "（平板）"
	}
	return label
}

func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// majorVersion 取版本号第一段（131.0.0.0 → 131）。没有版号返回空串。
func majorVersion(v string) string {
	v = strings.TrimSpace(v)
	cut := strings.IndexByte(v, '.')
	if cut >= 0 {
		v = v[:cut]
	}
	if !isDigits(v) {
		return ""
	}
	return v
}

// numericVersion 清洗系统版本号，只留数字与点（18_1 → 18.1）。
func numericVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "_", ".")
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.':
			b.WriteRune(r)
		default:
			return strings.Trim(b.String(), ".")
		}
	}
	// 最多保留两段（18.1.2 → 18.1），更细的补丁位对认设备没意义。
	parts := strings.Split(strings.Trim(b.String(), "."), ".")
	if len(parts) > 2 {
		return strings.Join(parts[:2], ".")
	}
	return strings.Trim(b.String(), ".")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
