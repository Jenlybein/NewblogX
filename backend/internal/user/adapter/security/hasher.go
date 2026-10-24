// Package security 是用户模块的加密适配器（密码哈希与校验）。
package security

import "myblogx/utils/pwd"

// Hasher 用 bcrypt 实现 app.PasswordHasher。
type Hasher struct{}

// Hash 生成 bcrypt 哈希。
func (Hasher) Hash(plain string) (string, error) { return pwd.GenerateFromPassword(plain) }

// Verify 校验明文与哈希是否匹配。
func (Hasher) Verify(hash, plain string) bool { return pwd.CompareHashAndPassword(hash, plain) }
