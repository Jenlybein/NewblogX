package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	corsAllowedOriginsEnv = "BLOGX_CORS_ALLOWED_ORIGINS"
	webSiteURLEnv         = "BLOGX_WEB_SITE_URL"
	adminSiteURLEnv       = "BLOGX_ADMIN_SITE_URL"
)

var defaultAllowedOrigins = []string{
	"http://localhost:3000",
	"http://127.0.0.1:3000",
	"http://localhost:3001",
	"http://127.0.0.1:3001",
	"http://localhost:5173",
	"http://127.0.0.1:5173",
	"https://blog.gentlybeing.cn",
	"https://blogx.gentlybeing.cn",
}

// CorsAllowedOrigins 返回当前进程允许的浏览器来源。
// BLOGX_CORS_ALLOWED_ORIGINS 一旦设置，会替换默认值；多个来源用英文逗号分隔。
// 默认值 = 内置本地开发来源 + 由站点地址（BLOGX_WEB_SITE_URL / BLOGX_ADMIN_SITE_URL）派生的来源，
// 所以换域名只改站点地址，不必再改这份代码。
func CorsAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv(corsAllowedOriginsEnv))
	if raw == "" {
		return normalizeAllowedOrigins(append(defaultAllowedOrigins, siteConfiguredOrigins()...))
	}

	return normalizeAllowedOrigins(strings.Split(raw, ","))
}

// IsAllowedOrigin 判断浏览器 Origin 是否允许访问 HTTP CORS 或 WebSocket。
func IsAllowedOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		// 非浏览器客户端、同源普通请求或内部探活经常没有 Origin，不应被 CORS 逻辑误伤。
		return true
	}

	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}

	for _, allowed := range CorsAllowedOrigins() {
		if normalizedOrigin == allowed {
			return true
		}
	}
	return false
}

// IsAllowedOriginRequest 供 WebSocket Upgrader 复用同一套 Origin 策略。
func IsAllowedOriginRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return IsAllowedOrigin(r.Header.Get("Origin"))
}

// siteConfiguredOrigins 把站点配置里的读者站地址与后台地址转成浏览器来源。
// 站点地址允许带 path（BLOGX_ADMIN_SITE_URL 生产值形如 https://域名/admin），
// 而 Origin 只能是 scheme://host，所以这里先剥掉 path / query / fragment 再规范化。
func siteConfiguredOrigins() []string {
	origins := make([]string, 0, 2)
	for _, key := range []string{webSiteURLEnv, adminSiteURLEnv} {
		if origin, ok := siteOriginFromURL(os.Getenv(key)); ok {
			origins = append(origins, origin)
		}
	}
	return origins
}

// siteOriginFromURL 从站点地址取出浏览器来源；非法值返回 false（宁可不放行，也不放宽比对）。
func siteOriginFromURL(rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || rawURL == "*" || strings.EqualFold(rawURL, "null") {
		return "", false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.Host == "" {
		return "", false
	}

	return scheme + "://" + strings.ToLower(parsed.Host), true
}

func normalizeAllowedOrigins(origins []string) []string {
	seen := make(map[string]struct{}, len(origins))
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		item, ok := normalizeOrigin(origin)
		if !ok {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeOrigin(origin string) (string, bool) {
	origin = strings.TrimSpace(origin)
	if origin == "" || origin == "*" || strings.EqualFold(origin, "null") {
		return "", false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}

	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}
