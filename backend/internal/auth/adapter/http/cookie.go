package http

import (
	"net/http"
	"time"
)

// refreshCookieName 与旧实现保持一致，否则老用户升级后会被强制重新登录。
const refreshCookieName = "refresh_token"

// setRefreshCookie 写刷新令牌 Cookie。
//
// HttpOnly + SameSite=Lax 是基线：JS 读不到（防 XSS 偷令牌），
// 跨站表单带不上（防 CSRF）。Secure 只在生产开启，否则本地 http 调试收不到 Cookie。
func setRefreshCookie(w http.ResponseWriter, token string, ttl time.Duration, secure bool) {
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(ttl / time.Second),
	})
}

// clearRefreshCookie 清除刷新令牌 Cookie。
func clearRefreshCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// refreshFromRequest 从请求里取刷新令牌。
func refreshFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
