package app

import (
	"context"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/domain"
)

// 本文件是"扩展登录方式"用例：邮箱验证码、QQ、注册、改密、重置、绑定。
//
// 依赖都做成窄端口（Directory / EmailCodes / OAuth）：认证模块只编排顺序，
// 用户数据怎么落库、邮件怎么发、QQ 怎么换 openid 全在适配器里。

// Directory 是账号目录能力（由 user 模块适配偶提供）。
type Directory interface {
	// FindByEmail 按邮箱取账号快照；不可登录时返回对应错误码。
	FindByEmail(ctx context.Context, email string) (Account, error)
	// RegisterByEmail 邮箱注册并建档。
	RegisterByEmail(ctx context.Context, email, plainPassword string) (Account, error)
	// FindOrCreateByOpenID 第三方登录：有则取、无则建档。
	FindOrCreateByOpenID(ctx context.Context, openID, nickname, avatar string) (Account, error)
	// ResetPasswordByEmail 用邮箱重置密码，返回受影响账号。
	ResetPasswordByEmail(ctx context.Context, email, newPassword string) (Account, error)
	// ChangePassword 已登录用户改密码（先验旧密码）。
	ChangePassword(ctx context.Context, userID uint64, oldPassword, newPassword string) error
	// BindEmail 给当前用户绑定邮箱。
	BindEmail(ctx context.Context, userID uint64, email string) error
}

// EmailCodes 是邮箱验证码的"生成 + 发送 + 存储"能力。
//
// 校验不在这里：它由 HTTP 中间件在进入用例前完成（一次性消费），
// 这样用例只关心"验证通过的邮箱要做什么"。
type EmailCodes interface {
	// Send 生成并发送验证码，返回验证码记录 ID。
	// kind：1 注册、2 重置密码、3 绑定邮箱、4 邮箱登录。
	// ip 用于按 IP 维度限流。
	Send(ctx context.Context, kind int8, email, ip string) (string, error)
}

// OAuth 是第三方登录的授权码换取能力。
type OAuth interface {
	Exchange(ctx context.Context, code string) (OAuthUser, error)
}

// OAuthUser 是第三方返回的最小用户信息。
type OAuthUser struct {
	OpenID   string
	Nickname string
	Avatar   string
}

// LoginWithEmail 邮箱验证码登录（验证码已由中间件校验）。
func (s *Service) LoginWithEmail(ctx context.Context, email string, meta domain.Meta) (*LoginResult, error) {
	if s.dir == nil {
		return nil, apperr.New(apperr.CodeAuthLoginDisabled)
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, apperr.New(apperr.CodeUserInvalidCredential)
	}
	account, err := s.dir.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, account, meta)
}

// LoginWithQQ QQ 授权码登录；首次登录自动建档。
func (s *Service) LoginWithQQ(ctx context.Context, code string, meta domain.Meta) (*LoginResult, error) {
	if s.oauth == nil || s.dir == nil {
		return nil, apperr.New(apperr.CodeAuthLoginDisabled)
	}
	if strings.TrimSpace(code) == "" {
		return nil, apperr.New(apperr.CodeUserOAuthFailed)
	}
	u, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, apperr.New(apperr.CodeUserOAuthFailed)
	}
	account, err := s.dir.FindOrCreateByOpenID(ctx, u.OpenID, u.Nickname, u.Avatar)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, account, meta)
}

// RegisterWithEmail 邮箱注册并直接登录。
func (s *Service) RegisterWithEmail(ctx context.Context, email, password string, meta domain.Meta) (*LoginResult, error) {
	if s.dir == nil {
		return nil, apperr.New(apperr.CodeUserRegisterDisabled)
	}
	if err := domain.ValidatePassword(password); err != nil {
		return nil, err
	}
	account, err := s.dir.RegisterByEmail(ctx, strings.TrimSpace(email), password)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, account, meta)
}

// ResetPasswordByEmail 用邮箱重置密码，并吊销该账号全部旧会话。
//
// 必须先吊销会话：改密后令牌版本 +1 只能让旧 access token 失效，
// 旧的 refresh token 还能换出新令牌，不吊销等于没改。
func (s *Service) ResetPasswordByEmail(ctx context.Context, email, newPassword string) error {
	if s.dir == nil {
		return apperr.New(apperr.CodeUserRegisterDisabled)
	}
	account, err := s.dir.ResetPasswordByEmail(ctx, strings.TrimSpace(email), newPassword)
	if err != nil {
		return err
	}
	return s.sessions.RevokeAllForUser(ctx, account.ID, s.clock.Now())
}

// ChangePassword 已登录用户改密码，成功后吊销全部会话（含当前设备）。
func (s *Service) ChangePassword(ctx context.Context, userID uint64, oldPassword, newPassword string) error {
	if s.dir == nil {
		return apperr.New(apperr.CodeUserRegisterDisabled)
	}
	if userID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if err := s.dir.ChangePassword(ctx, userID, oldPassword, newPassword); err != nil {
		return err
	}
	return s.sessions.RevokeAllForUser(ctx, userID, s.clock.Now())
}

// BindEmail 给当前用户绑定邮箱。
func (s *Service) BindEmail(ctx context.Context, userID uint64, email string) error {
	if s.dir == nil {
		return apperr.New(apperr.CodeUserRegisterDisabled)
	}
	if userID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	return s.dir.BindEmail(ctx, userID, strings.TrimSpace(email))
}

// SendEmailCode 发送邮箱验证码。
func (s *Service) SendEmailCode(ctx context.Context, kind int8, email, ip string) (string, error) {
	if s.codes == nil {
		return "", apperr.New(apperr.CodeUserMailSendFailed)
	}
	return s.codes.Send(ctx, kind, strings.TrimSpace(email), ip)
}
