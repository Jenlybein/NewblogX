// Package security 是认证模块的加密相关适配器：随机串与哈希。
//
// 令牌签发（JWT 载荷映射）放在装配层：它需要 models/enum，
// 而业务模块禁止 import models（archcheck R10 只放行 models/ctype）。
package security

import (
	"crypto/rand"
	"encoding/base64"

	"myblogx/internal/auth/domain"
	"myblogx/utils/hash"
)

// Secrets 生成刷新令牌原文，实现 domain.SecretGenerator。
type Secrets struct{}

var _ domain.SecretGenerator = (*Secrets)(nil)

// NewSecrets 构造随机串生成器。
func NewSecrets() *Secrets { return &Secrets{} }

// NewRefreshToken 返回 32 字节随机数的 base64url 编码。
func (*Secrets) NewRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Hasher 计算刷新令牌的 SHA256，实现 domain.Hasher。
type Hasher struct{}

var _ domain.Hasher = (*Hasher)(nil)

// NewHasher 构造哈希器。
func NewHasher() *Hasher { return &Hasher{} }

// SHA256 返回十六进制哈希。
func (*Hasher) SHA256(s string) string { return hash.SHA256Hash(s) }
