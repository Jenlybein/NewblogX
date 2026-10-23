package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

const corsAllowedOriginsEnv = "BLOGX_CORS_ALLOWED_ORIGINS"

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
func CorsAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv(corsAllowedOriginsEnv))
	if raw == "" {
		return normalizeAllowedOrigins(defaultAllowedOrigins)
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
