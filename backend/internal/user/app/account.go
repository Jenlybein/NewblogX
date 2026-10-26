package app

import (
	"context"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// 本文件是"账号生命周期"用例：注册、第三方建档、改密、重置、绑定邮箱。
//
// 与 service.go 里的资料读写分开：那些是"已登录用户在改自己"，
// 这里是"账号怎么出生、凭证怎么变"，调用方主要是认证模块。

// FindByEmailForAuth 邮箱验证码登录用的账号快照。
//
// 邮箱不存在统一返回"账号或密码错误"，不给外部枚举账号的机会。
func (s *Service) FindByEmailForAuth(ctx context.Context, email string) (domain.User, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}
	if err := domain.CanLogin(u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// FindOrCreateByOpenID 第三方登录：有则取、无则建档。
//
// 建档失败会回查一次：并发下另一个请求可能已经把同一个 openid 建好了，
// 这时"创建冲突"其实等于"登录成功"，不该报错。
func (s *Service) FindOrCreateByOpenID(ctx context.Context, openID, nickname, avatar string) (domain.User, error) {
	if strings.TrimSpace(openID) == "" {
		return domain.User{}, apperr.New(apperr.CodeUserOAuthFailed)
	}
	if u, err := s.users.FindByOpenID(ctx, openID); err == nil {
		if err := domain.CanLogin(u); err != nil {
			return domain.User{}, err
		}
		return u, nil
	}
	if s.usernameAlloc == nil {
		return domain.User{}, apperr.New(apperr.CodeUserRegisterDisabled)
	}

	oid := openID
	for i := 0; i < 5; i++ {
		name, err := s.usernameAlloc.Next()
		if err != nil {
			return domain.User{}, err
		}
		u := domain.User{
			Username:       name,
			Nickname:       nickname,
			Avatar:         avatar,
			OpenID:         &oid,
			Status:         domain.StatusActive,
			Role:           domain.RoleUser,
			RegisterSource: domain.SourceQQ,
			TokenVersion:   1,
			CreatedAt:      s.clock.Now(),
		}
		if err := s.users.Create(ctx, &u); err != nil {
			if got, findErr := s.users.FindByOpenID(ctx, openID); findErr == nil {
				if err := domain.CanLogin(got); err != nil {
					return domain.User{}, err
				}
				return got, nil
			}
			continue
		}
		s.initAccountDefaults(ctx, u.ID)
		return u, nil
	}
	return domain.User{}, apperr.New(apperr.CodeUserRegisterDisabled)
}

// RegisterByEmail 邮箱注册并立即建档。
func (s *Service) RegisterByEmail(ctx context.Context, email, plainPassword string) (domain.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return domain.User{}, apperr.New(apperr.CodeUserInvalidCredential)
	}
	if err := domain.ValidatePassword(plainPassword); err != nil {
		return domain.User{}, err
	}
	if s.hasher == nil || s.usernameAlloc == nil {
		return domain.User{}, apperr.New(apperr.CodeUserRegisterDisabled)
	}

	taken, err := s.users.ExistsEmail(ctx, email, "")
	if err != nil {
		return domain.User{}, err
	}
	if taken {
		return domain.User{}, apperr.New(apperr.CodeUserEmailTaken)
	}

	hash, err := s.hasher.Hash(plainPassword)
	if err != nil {
		return domain.User{}, err
	}

	mail := email
	for i := 0; i < 5; i++ {
		name, err := s.usernameAlloc.Next()
		if err != nil {
			return domain.User{}, err
		}
		u := domain.User{
			Username:       name,
			Nickname:       email,
			Avatar:         "xxx.png",
			Email:          &mail,
			PasswordHash:   hash,
			Status:         domain.StatusActive,
			Role:           domain.RoleUser,
			RegisterSource: domain.SourceEmail,
			TokenVersion:   1,
			CreatedAt:      s.clock.Now(),
		}
		if err := s.users.Create(ctx, &u); err != nil {
			// 邮箱并发抢注：另一个请求刚建好，直接报"已被使用"。
			if taken, _ := s.users.ExistsEmail(ctx, email, ""); taken {
				return domain.User{}, apperr.New(apperr.CodeUserEmailTaken)
			}
			continue
		}
		s.initAccountDefaults(ctx, u.ID)
		return u, nil
	}
	return domain.User{}, apperr.New(apperr.CodeUserRegisterDisabled)
}

// ResetPasswordByEmail 用邮箱验证码重置密码（不需要旧密码）。
//
// 返回用户快照是为了让认证模块拿到 ID 去吊销全部旧会话。
func (s *Service) ResetPasswordByEmail(ctx context.Context, email, newPassword string) (domain.User, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeUserNotFound)
	}
	if err := domain.ValidatePassword(newPassword); err != nil {
		return domain.User{}, err
	}
	if s.hasher == nil {
		return domain.User{}, apperr.New(apperr.CodeUserPasswordRequired)
	}
	if u.HasPassword() && s.hasher.Verify(u.PasswordHash, newPassword) {
		return domain.User{}, apperr.New(apperr.CodeUserPasswordSame)
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return domain.User{}, err
	}
	next := u.TokenVersion + 1
	if err := s.users.UpdatePassword(ctx, u.ID, hash, next); err != nil {
		return domain.User{}, err
	}
	u.PasswordHash = hash
	u.TokenVersion = next
	return u, nil
}

// ChangePassword 已登录用户改密码（要先验旧密码）。
func (s *Service) ChangePassword(ctx context.Context, id domain.UserID, oldPassword, newPassword string) error {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !u.HasPassword() || s.hasher == nil {
		return apperr.New(apperr.CodeUserPasswordRequired)
	}
	if !s.hasher.Verify(u.PasswordHash, oldPassword) {
		return apperr.New(apperr.CodeUserOldPasswordWrong)
	}
	if oldPassword == newPassword {
		return apperr.New(apperr.CodeUserPasswordSame)
	}
	if err := domain.ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, id, hash, u.TokenVersion+1)
}

// BindEmail 给当前用户绑定邮箱。
func (s *Service) BindEmail(ctx context.Context, id domain.UserID, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return apperr.New(apperr.CodeUserInvalidCredential)
	}
	taken, err := s.users.ExistsEmail(ctx, email, id)
	if err != nil {
		return err
	}
	if taken {
		return apperr.New(apperr.CodeUserEmailTaken)
	}
	return s.users.UpdateEmail(ctx, id, email)
}

// initAccountDefaults 建档三件套。
//
// 偏好/统计初始化失败只记日志：账号已经建好，缺这两行只影响展示，
// 回滚反而会让"注册成功但登录失败"。
func (s *Service) initAccountDefaults(ctx context.Context, id domain.UserID) {
	if err := s.settings.InitDefaults(ctx, id); err != nil {
		s.logError("初始化用户偏好失败: user=%s err=%v", id.String(), err)
	}
	if err := s.stats.Init(ctx, id); err != nil {
		s.logError("初始化用户统计失败: user=%s err=%v", id.String(), err)
	}
}
