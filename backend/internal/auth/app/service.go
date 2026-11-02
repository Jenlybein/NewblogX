// Package app 是认证与会话的用例层。
package app

import (
	"context"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"strings"
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
	ID           ctype.ID
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
//
// Infof 是 #99 加进来的：刷新路径要打印一次账号版本用于线上取证，
// 那是常规路径上的记录，不该占用 Warn 级别（Warn 留给"需要人看一眼"的异常）。
type Logger interface {
	Infof(format string, args ...any)
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

	// 以下依赖只在"扩展登录方式"（邮箱/QQ/改密/绑定）时使用，
	// 为空时对应用例会返回明确的错误，而不会 nil panic。
	dir   Directory
	codes EmailCodes
	oauth OAuth
}

// Option 是可选的装配项。
type Option func(*Service)

// WithDirectory 注入账号目录能力（注册、改密、绑定邮箱）。
func WithDirectory(d Directory) Option { return func(s *Service) { s.dir = d } }

// WithEmailCodes 注入邮箱验证码发送能力。
func WithEmailCodes(c EmailCodes) Option { return func(s *Service) { s.codes = c } }

// WithOAuth 注入第三方登录能力。
func WithOAuth(o OAuth) Option { return func(s *Service) { s.oauth = o } }

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
	extras ...Option,
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
	svc := &Service{
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
	for _, opt := range extras {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// LoginInput 是登录请求。
type LoginInput struct {
	Account  string
	Password string
	Meta     domain.Meta
	// RequireAdmin 表示这次登录来自管理后台，只有管理员才允许签发令牌。
	//
	// 检查放在**会话创建之前**（VerifyPassword 之后、startSession 之前），
	// 这样被拒的账号不会留下一条孤儿 session，refresh cookie 也不会被写出去。
	//
	// 注意它的作用边界：只能在"登录那一刻"拦住非管理员。Refresh 读的是 session 表、
	// 不读 claims，而零 DDL 口径下没法给 session 加一列记 scope —— 所以刷新之后
	// 拿到的就是普通令牌。真正的授权判断始终在 AdminMiddleware（每请求查库取 role）。
	RequireAdmin bool
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

	// 管理后台登录：密码已经验证通过了，这里再判断角色。
	// 放在 startSession 之前是刻意的 —— 否则会先建出一条 session、再返回"无权限"，
	// 库里留下一条没人用的会话记录，refresh cookie 也已经写出去了。
	if in.RequireAdmin && account.Role != int8(enum.RoleAdmin) {
		return nil, apperr.New(apperr.CodeAuthAdminRequired)
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

	// 同设备再次登录，把该设备的旧会话吊销掉（#131）。
	//
	// 设备列表是按设备聚合展示的，但聚合只是"看起来一条"——库里若还留着
	// 多条可换令牌的活会话，那些都是等价凭证。所以新会话落地后立刻挤掉旧的。
	// 顺序必须在 Create 之后：先建后吊销，新会话绝不会吊销到自己。
	s.revokeSameDeviceOlderSessions(ctx, session, now)

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

	account, err := s.creds.FindAccount(ctx, session.UserID.Uint64())
	if err != nil {
		return nil, apperr.New(apperr.CodeAuthRefreshInvalid)
	}

	// 诊断日志（#99）：区分"读到旧值"还是"签发丢值"。
	//
	// 线上实测"登录签发的令牌版本=1、刷新签发的=0"，而库里是 1。逐层排查
	// 已排除映射与签发链路丢字段，所以这里打印**会话所属账号读到的版本**。
	// 若这里打印的是 1 而签发出去是 0 → 丢值发生在 Issue；
	// 若这里就是 0 → 读到的是陈旧副本。
	s.logInfo("刷新读取账号版本(轮换前): session=%d user=%d token_version=%d",
		session.ID.Uint64(), session.UserID.Uint64(), account.TokenVersion)

	nextRefresh, err := s.secrets.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(s.opts.RefreshTTL)
	if err := s.sessions.Rotate(ctx, session.ID, s.hashRefresh(nextRefresh), expiresAt, meta); err != nil {
		return nil, err
	}

	// 签发前重新读取账号，版本只认这次读到的值（#99 的确定性加固）。
	//
	// 这条加固当初依赖一个隐式因果：上面那次读取可能命中查询缓存里的陈旧
	// 副本，而轮换写库会触发缓存整体失效，所以**再读一次**拿到的必然是当前值。
	// 那个因果已经不存在了 —— ORM 查询缓存插件已关闭（#132/#133），现在每次
	// 读都是真库值，不会读到一个"形状不对的陈旧副本"。
	//
	// 加固本身仍然保留，理由变了：它把"签发用的版本"与"最后一次读到的值"
	// 绑在一起，不再依赖任何中间层的隐式行为。缓存将来若以别的形式回来，
	// 这里依然是最靠得住的一道。
	//
	// 两次结果不一致时只告警、不拒绝：不一致本身是取证线索（说明读取路径上
	// 还有我们不知道的中间层）；而拒绝对用户没有任何好处 —— 他要的是一张
	// 能用的令牌，不是一次"你的缓存脏了"的报错。
	fresh, err := s.creds.FindAccount(ctx, session.UserID.Uint64())
	if err != nil {
		return nil, apperr.New(apperr.CodeAuthRefreshInvalid)
	}
	if fresh.TokenVersion != account.TokenVersion {
		s.logWarn("刷新前后读到的账号版本不一致(曾读到陈旧副本): session=%d user=%d before=%d after=%d",
			session.ID.Uint64(), session.UserID.Uint64(), account.TokenVersion, fresh.TokenVersion)
	}
	account = fresh
	s.logInfo("刷新签发账号版本: session=%d user=%d token_version=%d",
		session.ID.Uint64(), session.UserID.Uint64(), account.TokenVersion)

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
	if claims.SessionID.IsZero() {
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

	account, err := s.creds.FindAccount(ctx, claims.UserID.Uint64())
	if err != nil {
		return nil, apperr.New(apperr.CodeUserTokenInvalid)
	}
	if account.TokenVersion != claims.TokenVersion {
		// 版本号不一致说明改过密码或已被踢下线，旧令牌立即作废。
		return nil, apperr.New(apperr.CodeUserSessionExpired)
	}

	session, err := s.sessions.FindActive(ctx, ctype.NewID(claims.SessionID.Uint64()), claims.UserID.Uint64())
	if err != nil {
		return nil, apperr.New(apperr.CodeUserSessionExpired)
	}

	// 最后活跃时间用于"登录设备管理"显示，失败不影响鉴权结果。
	if err := s.sessions.Touch(ctx, session.ID, s.clock.Now()); err != nil {
		s.logError("更新会话活跃时间失败: session=%d err=%v", session.ID.Uint64(), err)
	}

	return &domain.Principal{
		UserID:    account.ID,
		SessionID: session.ID,
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
	if err := s.sessions.Revoke(ctx, ctype.NewID(principal.SessionID.Uint64()), now); err != nil {
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
func (s *Service) RevokeAll(ctx context.Context, userID ctype.ID) error {
	if userID.IsZero() {
		return nil
	}
	return s.sessions.RevokeAllForUser(ctx, userID.Uint64(), s.clock.Now())
}

// ListSessions 列出某用户当前所有有效会话，按设备聚合（登录设备管理）。
//
// 同一设备只返回一条（代表会话 + 首次登录 / 最近活跃 / 累计次数）。
// currentSessionID 是当前请求的会话，用于把「当前设备」标记落到那条会话上；
// 标记的判据是会话 ID，不是设备描述 —— 同设备多会话时描述会撞车。
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID ctype.ID) ([]domain.LoginDevice, error) {
	if userID.IsZero() {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	list, err := s.sessions.ListActive(ctx, userID.Uint64())
	if err != nil {
		return nil, err
	}
	return domain.AggregateDevices(list, currentSessionID), nil
}

// SessionActive 判断某个会话是否仍是该用户的有效会话（私信票据换发时用）。
//
// 不复用 ListSessions：列表已按设备聚合，同一设备的其它会话不会作为代表
// 出现在结果里，用聚合结果判断"这个会话还活着吗"会误判为失效。
func (s *Service) SessionActive(ctx context.Context, userID, sessionID uint64) (bool, error) {
	if userID == 0 || sessionID == 0 {
		return false, nil
	}
	if _, err := s.sessions.FindActive(ctx, ctype.NewID(sessionID), userID); err != nil {
		// 仓储对"查不到 / 已失效 / 不是你的"统一返回业务错误码；
		// 其它错误（数据库故障等）才需要向上暴露。
		if _, ok := apperr.CodeOf(err); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// revokeSameDeviceOlderSessions 吊销"同一设备"上除新会话之外的旧会话。
//
// 设备键忽略版本号（浏览器升级不该把一台设备拆成两条），具体口径见
// domain.DeviceGroupKey。
//
// 空 UA 时**不做**自动吊销：此时设备键会退化成同一个常量，照着它吊销会把
// 别的客户端会话一起挤掉。宁可不自动吊销，也不能误杀。
//
// 失败只记日志不返回错误：登录已经成功、会话已经落地，自动清理失败不该
// 让用户登不上；下次登录还会再尝试一次。
func (s *Service) revokeSameDeviceOlderSessions(ctx context.Context, fresh domain.Session, at time.Time) {
	if strings.TrimSpace(fresh.UA) == "" {
		return
	}

	key := domain.DeviceGroupKey(fresh)
	actives, err := s.sessions.ListActive(ctx, fresh.UserID.Uint64())
	if err != nil {
		s.logError("登录后列出同设备会话失败: user=%d err=%v", fresh.UserID.Uint64(), err)
		return
	}

	for _, old := range actives {
		if old.ID == fresh.ID {
			continue
		}
		if domain.DeviceGroupKey(old) != key {
			continue
		}
		if err := s.sessions.Revoke(ctx, old.ID, at); err != nil {
			s.logError("吊销同设备旧会话失败: session=%d err=%v", old.ID.Uint64(), err)
		}
	}
}

// RevokeSession 下线指定设备（登录设备管理里的"下线该设备"）。
//
// 先 FindActive 校验归属：只凭 sessionID 吊销等于让任何登录用户
// 踢掉别人的设备。校验不到就返回"已失效"，不区分"不存在"和"不是你的"。
//
// 列表是按设备聚合的，所以一次下线要吊销"同一设备分组"的全部会话：
// 只吊销代表会话的话，同设备的旧会话还在，刷新后设备会原样回来。
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uint64) error {
	if userID == 0 || sessionID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	target, err := s.sessions.FindActive(ctx, ctype.NewID(sessionID), userID)
	if err != nil {
		return err
	}

	key := domain.DeviceGroupKey(target)
	list, err := s.sessions.ListActive(ctx, userID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	for _, sess := range list {
		if domain.DeviceGroupKey(sess) != key {
			continue
		}
		if err := s.sessions.Revoke(ctx, sess.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// claimsOf 组装访问令牌载荷。
func (s *Service) claimsOf(account Account, sessionID domain.SessionID) domain.Claims {
	return domain.Claims{
		UserID:       account.ID,
		SessionID:    sessionID,
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

func (s *Service) logWarn(format string, args ...any) {
	if s.logger != nil {
		s.logger.Warnf(format, args...)
	}
}

func (s *Service) logInfo(format string, args ...any) {
	if s.logger != nil {
		s.logger.Infof(format, args...)
	}
}
