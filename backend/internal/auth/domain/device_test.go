package domain_test

import (
	"strings"
	"testing"

	"myblogx/internal/auth/domain"
)

// 真实样本：多浏览器 × 多系统。断言的是"给定 UA，对外呈现的设备信息是什么"，
// 不断言解析库内部结构——换解析实现时这些用例应当照旧通过。
const (
	uaChromeWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	uaEdgeWindows   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"
	uaFirefoxLinux  = "Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0"
	uaSafariMac     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15"
	uaChromeAndroid = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"
	uaSafariIPhone  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
	uaSafariIPad    = "Mozilla/5.0 (iPad; CPU OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
	uaOperaWindows  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 OPR/117.0.0.0"
	uaWeChatAndroid = "Mozilla/5.0 (Linux; Android 13; 2211133C Build/TKQ1.220829.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/116.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.40.2420(0x28002851) WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64"
	uaUCBrowser     = "Mozilla/5.0 (Linux; U; Android 12; zh-CN; V2134A) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.58 UCBrowser/15.5.0.1249 Mobile Safari/537.36"
	uaSamsung       = "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/121.0.0.0 Mobile Safari/537.36"
	uaGooglebot     = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
)

func TestParseUserAgent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ua   string
		want domain.Device
	}{
		{
			name: "Chrome / Windows",
			ua:   uaChromeWindows,
			want: domain.Device{Browser: "Chrome", BrowserVersion: "131", OS: "Windows", Type: domain.DeviceDesktop},
		},
		{
			name: "Edge / Windows",
			ua:   uaEdgeWindows,
			want: domain.Device{Browser: "Edge", BrowserVersion: "131", OS: "Windows", Type: domain.DeviceDesktop},
		},
		{
			name: "Firefox / Linux",
			ua:   uaFirefoxLinux,
			want: domain.Device{Browser: "Firefox", BrowserVersion: "133", OS: "Linux", Type: domain.DeviceDesktop},
		},
		{
			name: "Safari / macOS",
			ua:   uaSafariMac,
			want: domain.Device{Browser: "Safari", BrowserVersion: "18", OS: "macOS", Type: domain.DeviceDesktop},
		},
		{
			// Android 的 UA 里同时出现 Chrome，必须解析成桌面同名浏览器之外的"移动手机"。
			name: "Chrome / Android 手机",
			ua:   uaChromeAndroid,
			want: domain.Device{Browser: "Chrome", BrowserVersion: "131", OS: "Android", OSVersion: "14", Type: domain.DeviceMobile},
		},
		{
			name: "Safari / iPhone",
			ua:   uaSafariIPhone,
			want: domain.Device{Browser: "Safari", BrowserVersion: "18", OS: "iOS", OSVersion: "18.1", Type: domain.DeviceMobile},
		},
		{
			name: "Safari / iPad",
			ua:   uaSafariIPad,
			want: domain.Device{Browser: "Safari", BrowserVersion: "18", OS: "iOS", OSVersion: "18.1", Type: domain.DeviceTablet},
		},
		{
			// 微信 Android 的 UA 末尾才是 MicroMessenger，不能因为前面有 Chrome 就当 Chrome。
			name: "微信 / Android",
			ua:   uaWeChatAndroid,
			want: domain.Device{Browser: "WeChat", BrowserVersion: "8", OS: "Android", OSVersion: "13", Type: domain.DeviceMobile},
		},
		{
			name: "UC 浏览器 / Android",
			ua:   uaUCBrowser,
			want: domain.Device{Browser: "UC Browser", BrowserVersion: "15", OS: "Android", OSVersion: "12", Type: domain.DeviceMobile},
		},
		{
			name: "三星浏览器 / Android",
			ua:   uaSamsung,
			want: domain.Device{Browser: "Samsung Internet", BrowserVersion: "25", OS: "Android", OSVersion: "14", Type: domain.DeviceMobile},
		},
		{
			name: "Opera / Windows",
			ua:   uaOperaWindows,
			want: domain.Device{Browser: "Opera", BrowserVersion: "117", OS: "Windows", Type: domain.DeviceDesktop},
		},
		{
			name: "搜索引擎爬虫",
			ua:   uaGooglebot,
			want: domain.Device{Type: domain.DeviceBot},
		},
		{
			name: "空 UA",
			ua:   "",
			want: domain.Device{Type: domain.DeviceUnknown},
		},
		{
			name: "空白 UA",
			ua:   "   ",
			want: domain.Device{Type: domain.DeviceUnknown},
		},
		{
			name: "无法识别的客户端串",
			ua:   "test-agent",
			want: domain.Device{Type: domain.DeviceUnknown},
		},
		{
			name: "非 ASCII 异常 UA",
			ua:   "你好，世界",
			want: domain.Device{Type: domain.DeviceUnknown},
		},
		{
			name: "超长异常 UA",
			ua:   strings.Repeat("A", 4096),
			want: domain.Device{Type: domain.DeviceUnknown},
		},
		{
			name: "控制字符 UA",
			ua:   "\x00\x01\x02Mozilla/5.0\x03",
			want: domain.Device{Type: domain.DeviceUnknown},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// 异常 UA 不许 panic、不许返回半截脏数据。
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("解析 UA 时 panic: %v", r)
				}
			}()
			got := domain.ParseUserAgent(c.ua)
			if got != c.want {
				t.Fatalf("UA=%q\n解析结果 got=%+v\nwant=%+v", c.ua, got, c.want)
			}
		})
	}
}

// 同一 UA 多次解析必须得到同一结果（列表按设备描述聚合的前提）。
func TestParseUserAgentStable(t *testing.T) {
	t.Parallel()
	const ua = uaChromeWindows
	for i := 0; i < 5; i++ {
		first := domain.ParseUserAgent(ua)
		second := domain.ParseUserAgent(ua)
		if first != second {
			t.Fatalf("第 %d 次解析结果不稳定: %+v vs %+v", i, first, second)
		}
	}
}

// 设备键只由"浏览器 + 系统 + 类型"决定：小版本升级不该把同一台设备拆成两条。
func TestDeviceKeyIgnoresPatchVersions(t *testing.T) {
	t.Parallel()

	chrome120 := domain.ParseUserAgent(strings.Replace(uaChromeWindows, "131.0.0.0", "120.0.0.0", 1))
	chrome131 := domain.ParseUserAgent(uaChromeWindows)
	if chrome120.Key() != chrome131.Key() {
		t.Fatalf("同设备不同版本应同键: %q vs %q", chrome120.Key(), chrome131.Key())
	}

	if chrome131.Key() == domain.ParseUserAgent(uaFirefoxLinux).Key() {
		t.Fatal("不同浏览器不应同键")
	}
	if chrome131.Key() == domain.ParseUserAgent(strings.Replace(uaChromeWindows, "Windows", "Linux", 1)).Key() {
		t.Fatal("不同操作系统不应同键")
	}
	if domain.ParseUserAgent(uaChromeAndroid).Key() == chrome131.Key() {
		t.Fatal("手机与桌面不应同键")
	}
}

// Label 是给用户看的人话，不是 UA 原文。
func TestDeviceLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ua   string
		want string
	}{
		{"Chrome/Windows", uaChromeWindows, "Chrome 131 · Windows"},
		{"Safari/iPhone", uaSafariIPhone, "Safari 18 · iOS 18.1（手机）"},
		{"Safari/iPad", uaSafariIPad, "Safari 18 · iOS 18.1（平板）"},
		{"空 UA", "", "未知设备"},
		{"异常 UA", "test-agent", "未知设备"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ParseUserAgent(c.ua).Label(); got != c.want {
				t.Fatalf("Label got=%q want=%q", got, c.want)
			}
		})
	}
}
