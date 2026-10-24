package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// VerifyPassword 实现 CredentialPort。
//
// 三种失败（用户不存在 / 没设密码 / 密码错）统一返回 CodeUserInvalidCredential：
// 分开返回等于给攻击者一个账号枚举接口。
func (s *Service) VerifyPassword(ctx context.Context, identifier, plain string) (domain.User, error) {
	if identifier == "" || plain == "" {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}

	// 用户名和邮箱都可以作为登录名，先按用户名查、查不到再按邮箱查。
	u, err := s.users.FindByUsername(ctx, identifier)
	if err != nil {
		u, err = s.users.FindByEmail(ctx, identifier)
	}
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}

	// QQ 登录建档的账号可能没有密码，此时不允许密码登录。
	if !u.HasPassword() || s.hasher == nil {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}
	if !s.hasher.Verify(u.PasswordHash, plain) {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}

	// 密码对了才检查状态：状态错误要给用户明确提示（禁用/封禁），
	// 但这只发生在密码正确之后，泄露风险可接受。
	if err := domain.CanLogin(u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// FindForAuth 实现 CredentialPort。
func (s *Service) FindForAuth(ctx context.Context, id domain.UserID) (domain.User, error) {
	if id.IsZero() {
		return domain.User{}, apperr.New(apperr.CodeUserTokenInvalid)
	}
	return s.users.FindByID(ctx, id)
}
