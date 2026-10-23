package user_service

import (
	"crypto/rand"
	"encoding/base64"
	"myblogx/models"
	"myblogx/utils/hash"
	"myblogx/utils/jwts"
	"net/http"
	"time"
)

// CreateLoginTokens 创建登录成功后的双令牌（AccessToken + RefreshToken）同时生成并持久化用户会话到数据库
func CreateLoginTokens(deps Deps, user *models.UserModel, meta SessionMeta) (accessToken string, refreshToken string, session *models.UserSessionModel, err error) {
	if deps.DB == nil {
		return "", "", nil, ErrAuthInvalid
	}
	// 生成随机安全的刷新令牌
	refreshToken, err = generateRefreshToken()
	if err != nil {
		return "", "", nil, err
	}

	now := time.Now()
	// 构造会话模型（存储哈希后的refresh_token，不存原文）
	session = &models.UserSessionModel{
		UserID:           user.ID,                       // 所属用户ID
		RefreshTokenHash: hash.SHA256Hash(refreshToken), // 刷新令牌哈希（SHA256）
		IP:               meta.IP,                       // 登录IP
		Addr:             meta.Addr,                     // 登录地址
		UA:               meta.UA,                       // 登录设备UA
		LastSeenAt:       &now,                          // 最后活跃时间
		ExpiresAt:        now.Add(time.Duration(deps.JWT.RefreshExpire) * time.Hour),
	}

	// 将会话写入数据库
	if err = deps.DB.Create(session).Error; err != nil {
		return "", "", nil, err
	}

	// 生成JWT访问令牌（短期）
	accessToken, err = jwts.GetToken(deps.JWT, jwts.Claims{
		UserID:       user.ID,
		SessionID:    session.ID,
		TokenVersion: user.TokenVersion, // 令牌版本：用于全局吊销旧令牌
		Username:     user.Username,
		Role:         user.Role,
	})
	if err != nil {
		return "", "", nil, err
	}
	return accessToken, refreshToken, session, nil
}

// RefreshTokens 刷新令牌核心方法
// 使用旧RefreshToken换取新的AccessToken + 新RefreshToken（轮换机制，更安全）
func RefreshTokens(deps Deps, refreshToken string, meta SessionMeta) (accessToken string, newRefreshToken string, user *models.UserModel, session *models.UserSessionModel, err error) {
	if deps.DB == nil {
		return "", "", nil, nil, ErrAuthInvalid
	}
	if refreshToken == "" {
		return "", "", nil, nil, ErrRefreshTokenRequired
	}

	now := time.Now()
	// 根据refresh_token哈希查询有效会话
	var currentSession models.UserSessionModel
	if err = deps.DB.
		Where("refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", hash.SHA256Hash(refreshToken), now).
		Take(&currentSession).Error; err != nil {
		return "", "", nil, nil, ErrRefreshTokenInvalid
	}

	// 查询会话对应用户
	var currentUser models.UserModel
	if err = deps.DB.Take(&currentUser, currentSession.UserID).Error; err != nil {
		return "", "", nil, nil, ErrRefreshTokenInvalid
	}
	// 校验用户状态
	if err = currentUser.ValidateUserStatus(); err != nil {
		return "", "", nil, nil, err
	}

	// 生成新的刷新令牌（轮换机制，每次刷新都更换refresh_token）
	newRefreshToken, err = generateRefreshToken()
	if err != nil {
		return "", "", nil, nil, err
	}

	// 更新会话信息：新令牌哈希、新过期时间、最新IP/UA/地址
	updates := map[string]any{
		"refresh_token_hash": hash.SHA256Hash(newRefreshToken),
		"expires_at":         now.Add(time.Duration(deps.JWT.RefreshExpire) * time.Hour),
		"last_seen_at":       now,
		"ip":                 meta.IP,
		"addr":               meta.Addr,
		"ua":                 meta.UA,
	}
	if err = deps.DB.Model(&currentSession).Updates(updates).Error; err != nil {
		return "", "", nil, nil, err
	}

	// 生成新的访问令牌
	accessToken, err = jwts.GetToken(deps.JWT, jwts.Claims{
		UserID:       currentUser.ID,
		SessionID:    currentSession.ID,
		TokenVersion: currentUser.TokenVersion,
		Username:     currentUser.Username,
		Role:         currentUser.Role,
	})
	if err != nil {
		return "", "", nil, nil, err
	}

	return accessToken, newRefreshToken, &currentUser, &currentSession, nil
}

// SetRefreshTokenCookie 将刷新令牌写入HttpOnly Cookie
// 安全策略：HttpOnly + 生产环境启用Secure + SameSiteLax
func SetRefreshTokenCookie(w http.ResponseWriter, refreshToken string, deps Deps) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,                     // 禁止JS读取，防止XSS窃取
		SameSite: http.SameSiteLaxMode,     // 防止CSRF
		Secure:   isSecureCookie(deps.Env), // 生产环境HTTPS才发送
		MaxAge:   int(time.Duration(deps.JWT.RefreshExpire) * time.Hour / time.Second),
	})
}

// ClearRefreshTokenCookie 清除刷新令牌Cookie
// 用于退出登录
func ClearRefreshTokenCookie(w http.ResponseWriter, deps Deps) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureCookie(deps.Env),
		MaxAge:   -1, // 立即过期
		Expires:  time.Unix(0, 0),
	})
}

// GetRefreshTokenByGin 从Gin请求中获取refresh_token cookie
func GetRefreshTokenByRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// isSecureCookie 判断是否启用Secure Cookie
// 生产环境才开启（HTTPS）
func isSecureCookie(env string) bool {
	return env == "prod"
}

// generateRefreshToken 生成安全的随机刷新令牌
// 32字节随机数 → RawURLEncoding 字符串
func generateRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
