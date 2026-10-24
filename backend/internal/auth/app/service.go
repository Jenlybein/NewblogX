// Package app 是认证与会话的用例层。
package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/domain"
)

// Account 是认证需要的用户快照。
//
// 刻意不用 user 模块的实体：认证只需要这五个字段，把整个 User（含邮箱、
// 简介、偏好）传进来既没必要，也会让两个模块的实体耦在一起。
// 由装配层把 user/app 的结果转换成本结构。
type Account struct {
	ID           uint64
	Username     string
	Role         int8
	TokenVersion uint32
}

// Credentials 是用户模块提供的凭证能力（定义在消费方，实现由装配层适配）。
type Credentials interface {
	// VerifyPassword 校验「用户名或邮箱 + 密码」；失败返回 apperr 错误码。
	VerifyPassword(ctx context.Context, identifier, plain string) (Account, error)
	// FindAccount 按 ID 查账号快照，供令牌校验使用。
	FindAccount(ctx context.Context, userID uint64) (Account, error)
}

// Logger 是窄日志接口。
type Logger interface {
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Options 是令牌有效期等策略。
type Options struct {
	// AccessTTL 访问令牌有效期（短，默认 2 小时）。
	AccessTTL time.Duration
	// RefreshTTL 会话/刷新令牌有效期（长，默认 30 天）。
	RefreshTTL time.Duration
}

// withDefaults 补齐零值，防止"忘了配"导致令牌永不过期。
func (o Options) withDefaults() Options {
	if o.AccessTTL <= 0 {
		o.AccessTTL = 2 * time.Hour
	}
	if o.RefreshTTL <= 0 {
		o.RefreshTTL = 30 * 24 * time.Hour
	}
	return o
}

// Service 是认证服务。
type Service struct {
	sessions  domain.SessionRepository
	issuer    domain.TokenIssuer
	secrets   domain.SecretGenerator
	hasher    domain.Hasher
	blacklist domain.TokenBlacklist
	guard     domain.LoginGuard
	creds     Credentials
	clock     domain.Clock
	logger    Logger
	opts      Options
}

// New 构造认证服务。
func New(
	sessions domain.SessionRepository,
	issuer domain.TokenIssuer,
	secrets domain.SecretGenerator,
	hasher domain.Hasher,
	blacklist domain.TokenBlacklist,
	guard domain.LoginGuard,
	creds Credentials,
	clock domain.Clock,
	logger Logger,
	opts Options,
) *Service {
	if sessions == nil {
		panic("auth/app: 会话仓储未注入")
	}
	if issuer == nil {
		panic("auth/app: 令牌签发器未注入")
	}
	if secrets == nil {
		panic("auth/app: 随机串生成器未注入")
	}
	if creds == nil {
		panic("auth/app: 凭证能力未注入")
	}
	if clock == nil {
		panic("auth/app: 时钟未注入")
	}
	return &Service{
		sessions:  sessions,
		issuer:    issuer,
		secrets:   secrets,
		hasher:    hasher,
		blacklist: blacklist,
		guard:     guard,
		creds:     creds,
		clock:     clock,
		logger:    logger,
		opts:      opts.withDefaults(),
	}
}

// LoginInput 是登录请求。
type LoginInput struct {
	Account  string
	Password string
	Meta     domain.Meta
}

// LoginResult 是登录/刷新的结果。
type LoginResult struct {
	Tokens  domain.Tokens
	Session domain.Session
	Account Account
}

// Login 用账号密码登录。
//
// 顺序是刻意的：
//  1. 先看失败次数（否则爆破没有成本）
//  2. 再验密码（失败记一次）
//  3. 成功才清零计数
//  4. 最后建会话 + 签令牌
//
// 任何一步失败都不建会话，避免留下"登录失败但会话表在涨"的垃圾。
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	if err := domain.ValidateIdentifier(in.Account); err != nil {
		return nil, err
	}
	if in.Password == "" {
		return nil, apperr.New(apperr.CodeUserInvalidCredential)
	}

	if s.guard != nil {
		allowed, err := s.guard.Allowed(ctx, in.Account, in.Meta.IP)
		if err != nil {
			// 限流器本身故障时放行：宁可短暂失去限流，也不能让所有人登不进。
			s.logError("登录限流检查失败: account=%s err=%v", in.Account, err)
		} else if !allowed {
			return nil, apperr.New(apperr.CodeAuthTooFrequent)
		}
	}

	account, err := s.creds.VerifyPassword(ctx, in.Account, in.Password)
	if err != nil {
		if s.guard != nil {
			if ferr := s.guard.RecordFailure(ctx, in.Account, in.Meta.IP); ferr != nil {
				s.logError("记录登录失败次数出错: account=%s err=%v", in.Account, ferr)
			}
		}
		return nil, err
	}

	if s.guard != nil {
		if err := s.guard.Reset(ctx, in.Account, in.Meta.IP); err != nil {
			s.logError("重置登录失败次数出错: account=%s err=%v", in.Account, err)
		}
	}

	return s.startSession(ctx, account, in.Meta)
}

// startSession 建会话并签发双令牌。
func (s *Service) startSession(ctx context.Context, account Account, meta domain.Meta) (*LoginResult, error) {
	now := s.clock.Now()
	refresh, err := s.secrets.NewRefreshToken()
	if err != nil {
		return nil, err
	}

	session := domain.Session{
		UserID:           account.ID,
		RefreshTokenHash: s.hashRefresh(refresh),
		IP:               meta.IP,
		Addr:             meta.Addr,
		UA:               meta.UA,
		LastSeenAt:       &now,
		ExpiresAt:        now.Add(s.opts.RefreshTTL),
	}
	if err := s.sessions.Create(ctx, &session); err != nil {
		return nil, err
	}

	access, err := s.issuer.Issue(s.claimsOf(account, session.ID))
	if err != nil {
		// 会话已经落库但令牌签不出来：把会话吊销掉，避免留下一条永远换不到令牌的记录。
		if rerr := s.sessions.Revoke(ctx, session.ID, now); rerr != nil {
			s.logError("签发令牌失败后吊销会话也失败: session=%d err=%v", session.ID.Uint64(), rerr)
		}
		return nil, err
	}

	return &LoginResult{
		Tokens:  domain.Tokens{Access: access, Refresh: refresh},
		Session: session,
		Account: account,
	}, nil
}

// Refresh 用刷新令牌换新的双令牌（轮换）。
//
// 每次刷新都换新的 refresh token：旧的立即失效。这样一条泄露的刷新令牌
// 最多用一次，而且如果攻击者用了，正常用户下次刷新会失败、从而感知到异常。
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta domain.Meta) (*LoginResult, error) {
	if refreshToken == "" {
		return nil, apperr.New(apperr.CodeAuthRefreshInvalid)
	}

	session, err := s.sessions.FindByRefreshHash(ctx, s.hashRefresh(refreshToken))
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if err := session.EnsureActive(now); err != nil {
		return nil, err
	}

	account, err := s.creds.FindAccount(ctx, session.UserID)
	if err != nil {
		return nil, apperr.New(apperr.CodeAuthRefreshInvalid)
	}

	nextRefresh, err := s.secrets.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(s.opts.RefreshTTL)
	if err := s.sessions.Rotate(ctx, session.ID, s.hashRefresh(nextRefresh), expiresAt, meta); err != nil {
		return nil, err
	}

	access, err := s.issuer.Issue(s.claimsOf(account, session.ID))
	if err != nil {
		return nil, err
	}

	session.RefreshTokenHash = s.hashRefresh(nextRefresh)
	session.ExpiresAt = expiresAt
	return &LoginResult{
		Tokens:  domain.Tokens{Access: access, Refresh: nextRefresh},
		Session: session,
		Account: account,
	}, nil
}

// Authenticate 校验访问令牌，返回操作者身份。
//
// 五步全部通过才算登录态有效，任何一步失败都返回"重新登录"：
// 令牌能解析 → 不在黑名单 → 用户还在 → 令牌版本一致 → 会话未吊销。
func (s *Service) Authenticate(ctx context.Context, accessToken string) (*domain.Principal, error) {
	if accessToken == "" {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}

	claims, err := s.issuer.Parse(accessToken)
	if err != nil {
		return nil, apperr.New(apperr.CodeUserTokenInvalid)
	}
	if claims.SessionID == 0 {
		return nil, apperr.New(apperr.CodeUserTokenInvalid)
	}

	if s.blacklist != nil {
		revoked, err := s.blacklist.Contains(ctx, accessToken)
		if err != nil {
			// 黑名单查不了时按"已注销"处理：宁可多要求一次登录，
			// 也不能让已注销的令牌继续用。
			s.logError("查询令牌黑名单失败: err=%v", err)
			return nil, apperr.New(apperr.CodeUserSessionExpired)
		}
		if revoked {
			return nil, apperr.New(apperr.CodeUserSessionExpired)
		}
	}

	account, err := s.creds.FindAccount(ctx, claims.UserID)
	if err != nil {
		return nil, apperr.New(apperr.CodeUserTokenInvalid)
	}
	if account.TokenVersion != claims.TokenVersion {
		// 版本号不一致说明改过密码或已被踢下线，旧令牌立即作废。
		return nil, apperr.New(apperr.CodeUserSessionExpired)
	}

	session, err := s.sessions.FindActive(ctx, domain.SessionID(claims.SessionID), claims.UserID)
	if err != nil {
		return nil, apperr.New(apperr.CodeUserSessionExpired)
	}

	// 最后活跃时间用于"登录设备管理"显示，失败不影响鉴权结果。
	if err := s.sessions.Touch(ctx, session.ID, s.clock.Now()); err != nil {
		s.logError("更新会话活跃时间失败: session=%d err=%v", session.ID.Uint64(), err)
	}

	return &domain.Principal{
		UserID:    account.ID,
		SessionID: session.ID.Uint64(),
		Username:  account.Username,
		Role:      account.Role,
	}, nil
}

// Logout 注销当前会话。
//
// 两件事都要做：吊销服务端会话（刷新令牌失效）+ 把当前访问令牌加入黑名单
// （它还没过期，不拉黑就还能用）。
func (s *Service) Logout(ctx context.Context, principal *domain.Principal, accessToken string) error {
	if principal == nil {
		return apperr.New(apperr.CodeAuthRequired)
	}
	now := s.clock.Now()
	if err := s.sessions.Revoke(ctx, domain.SessionID(principal.SessionID), now); err != nil {
		return err
	}
	if s.blacklist != nil && accessToken != "" {
		// 黑名单只需要保留到令牌自然过期，过长的 TTL 纯属浪费内存。
		if err := s.blacklist.Add(ctx, accessToken, s.opts.AccessTTL); err != nil {
			s.logError("令牌加入黑名单失败: err=%v", err)
		}
	}
	return nil
}

// RevokeAll 吊销某用户的全部会话（改密码、封号、踢下线时调用）。
func (s *Service) RevokeAll(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return nil
	}
	return s.sessions.RevokeAllForUser(ctx, userID, s.clock.Now())
}

// ListSessions 列出某用户当前所有有效会话（登录设备管理）。
func (s *Service) ListSessions(ctx context.Context, userID uint64) ([]domain.Session, error) {
	if userID == 0 {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	return s.sessions.ListActive(ctx, userID)
}

// claimsOf 组装访问令牌载荷。
func (s *Service) claimsOf(account Account, sessionID domain.SessionID) domain.Claims {
	return domain.Claims{
		UserID:       account.ID,
		SessionID:    sessionID.Uint64(),
		TokenVersion: account.TokenVersion,
		Username:     account.Username,
		Role:         account.Role,
	}
}

// hashRefresh 计算刷新令牌的存储哈希。
//
// 没注入 hasher 时退化成原文：仅用于测试装配缺失的场景，
// 生产装配必须注入（否则数据库泄露 = 会话泄露）。
func (s *Service) hashRefresh(raw string) string {
	if s.hasher == nil {
		return raw
	}
	return s.hasher.SHA256(raw)
}

func (s *Service) logError(format string, args ...any) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}
