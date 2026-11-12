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
	// FindOrCreateByPhone 手机号登录：有则取、无则建档。
	FindOrCreateByPhone(ctx context.Context, phone string) (Account, error)
	// SetPassword 给尚无密码的账号设置首个密码（已有密码时返回对应错误）。
	SetPassword(ctx context.Context, userID uint64, newPassword string) error
	// PhoneOf 返回账号已绑定的手机号；未绑定时返回空串。
	PhoneOf(ctx context.Context, userID uint64) (string, error)
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

// SmsCodes 是短信验证码的"发送 + 核验"能力。
//
// 与 EmailCodes 的差别值得注意：邮件验证码由我们生成并存在 Redis 里，
// 所以"核验"可以放在 HTTP 中间件里做（一次性消费）；短信验证码由阿里云
// 生成、也必须由阿里云核验，所以这两个方法都落在这里，中间件只负责
// "问一句对不对"并把手机号放进上下文。
type SmsCodes interface {
	// Send 发送验证码。kind 的取值与 platform/sms 的 Kind 一一对齐，
	// 这里用 int8 是为了不让用例层 import 平台包（与 EmailCodes 同一口径）。
	Send(ctx context.Context, kind int8, phone, ip string) error
	// Check 核验验证码；(false, nil) 表示码错或过期。
	Check(ctx context.Context, phone, code string) (bool, error)
}

// OAuthUser 是第三方返回的最小用户信息。
type OAuthUser struct {
	OpenID   string
	Nickname string
	Avatar   string
}

// LoginWithPhone 手机号 + 短信验证码登录；首次登录自动建档。
//
// 验证码已经由 HTTP 中间件向阿里云核验过，并且手机号是中间件从核验结果里
// 放回上下文的（不是直接信任请求体），所以这里只回答一个问题：这个号对应谁。
func (s *Service) LoginWithPhone(ctx context.Context, phone string, meta domain.Meta) (*LoginResult, error) {
	if s.dir == nil {
		return nil, apperr.New(apperr.CodeAuthLoginDisabled)
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, apperr.New(apperr.CodeUserPhoneInvalid)
	}
	account, err := s.dir.FindOrCreateByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, account, meta)
}

// SetPassword 给"当前没有密码"的账号设置首个密码。
//
// 存在的理由：手机号/第三方建档的账号没有密码，而改密接口强制验旧密码，
// 于是他们永远拿不到密码、也就永远用不了「用户名 + 密码」登录。
//
// 两道防线，缺一不可：
//  1. 必须已登录（userID 来自会话，不是请求体）；
//  2. 必须对**本账号已绑定的手机号**再做一次短信复验——只凭"已登录"就
//     允许设密码，会让一个被盗的会话把临时凭证升级成永久密码凭证。
//
// 已有密码的账号必须走 ChangePassword，否则这里就是免旧密码改密的后门。
func (s *Service) SetPassword(ctx context.Context, userID uint64, phone, code, newPassword string) error {
	if s.dir == nil {
		return apperr.New(apperr.CodeUserRegisterDisabled)
	}
	if userID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if err := domain.ValidatePassword(newPassword); err != nil {
		return err
	}

	bound, err := s.dir.PhoneOf(ctx, userID)
	if err != nil {
		return err
	}
	// 号码必须与绑定的一致：拿别人的号收码来改自己的密码没有意义，
	// 但把校验写清楚能挡住"改了手机号参数"这类误用。
	if bound == "" || bound != strings.TrimSpace(phone) {
		return apperr.New(apperr.CodeUserPhoneInvalid)
	}

	ok, err := s.CheckSmsCode(ctx, phone, code)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.New(apperr.CodeUserCaptchaInvalid)
	}

	return s.dir.SetPassword(ctx, userID, newPassword)
}

// SendSmsCode 发送短信验证码。
func (s *Service) SendSmsCode(ctx context.Context, kind int8, phone, ip string) error {
	if s.smsCodes == nil {
		return apperr.New(apperr.CodeUserSmsSendFailed)
	}
	return s.smsCodes.Send(ctx, kind, strings.TrimSpace(phone), ip)
}

// CheckSmsCode 核验短信验证码。
func (s *Service) CheckSmsCode(ctx context.Context, phone, code string) (bool, error) {
	if s.smsCodes == nil {
		return false, apperr.New(apperr.CodeUserSmsSendFailed)
	}
	return s.smsCodes.Check(ctx, strings.TrimSpace(phone), strings.TrimSpace(code))
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
	return s.sessions.RevokeAllForUser(ctx, account.ID.Uint64(), s.clock.Now())
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
