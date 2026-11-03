package middleware_test

import (
	"testing"

	"myblogx/internal/platform/http/middleware"
)

func TestOriginAllowlistIncludesWebSiteOriginFromConfig(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("BLOGX_WEB_SITE_URL", "https://gentlybeing.cn")
	t.Setenv("BLOGX_ADMIN_SITE_URL", "")

	if !middleware.IsAllowedOrigin("https://gentlybeing.cn") {
		t.Fatal("生产站点地址（BLOGX_WEB_SITE_URL）应进入默认 Origin 白名单")
	}
}

func TestOriginAllowlistStripsPathFromAdminSiteOrigin(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("BLOGX_WEB_SITE_URL", "")
	t.Setenv("BLOGX_ADMIN_SITE_URL", "https://gentlybeing.cn/admin")

	if !middleware.IsAllowedOrigin("https://gentlybeing.cn") {
		t.Fatal("带 path 的后台地址（BLOGX_ADMIN_SITE_URL）应剥离 path 后进入白名单")
	}
	if middleware.IsAllowedOrigin("https://gentlybeing.cn/admin") {
		t.Fatal("带 path 的 URL 不是合法 Origin，不应被放行")
	}
}

func TestOriginAllowlistExplicitEnvReplacesSiteDerivedOrigins(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "https://front.example.com")
	t.Setenv("BLOGX_WEB_SITE_URL", "https://site.example.com")
	t.Setenv("BLOGX_ADMIN_SITE_URL", "https://site.example.com/admin")

	if !middleware.IsAllowedOrigin("https://front.example.com") {
		t.Fatal("显式设置的允许来源应始终被放行")
	}
	if middleware.IsAllowedOrigin("https://site.example.com") {
		t.Fatal("显式设置允许来源后应整体替换白名单，站点派生来源不应再混入")
	}
	if middleware.IsAllowedOrigin("http://localhost:3000") {
		t.Fatal("显式设置允许来源后内置默认值不应再生效")
	}
}

func TestOriginAllowlistRejectsUnsafeOriginsDespiteSiteConfig(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("BLOGX_WEB_SITE_URL", "https://gentlybeing.cn")
	t.Setenv("BLOGX_ADMIN_SITE_URL", "https://gentlybeing.cn/admin")

	for _, origin := range []string{
		"*",
		"null",
		"file://local/index.html",
		"https://evil.example",
		"https://gentlybeing.cn.evil.example",
		"https://gentlybeing.cn/admin",
		"http://gentlybeing.cn",
	} {
		if middleware.IsAllowedOrigin(origin) {
			t.Fatalf("不安全或非白名单来源应被拒绝: %q", origin)
		}
	}
}

func TestOriginAllowlistIgnoresInvalidSiteConfig(t *testing.T) {
	t.Setenv("BLOGX_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("BLOGX_WEB_SITE_URL", "*")
	t.Setenv("BLOGX_ADMIN_SITE_URL", "javascript://alert")

	for _, origin := range []string{"*", "null", "javascript://alert"} {
		if middleware.IsAllowedOrigin(origin) {
			t.Fatalf("站点配置非法时不应派生出可放行来源: %q", origin)
		}
	}
	if !middleware.IsAllowedOrigin("http://localhost:3000") {
		t.Fatal("站点配置非法时内置默认白名单应继续生效")
	}
}
