// Package apptest 提供认证模块的有状态测试替身。
//
// 与 user 模块的 apptest 不同，这里的替身要能表达"时间流逝"和"并发重复刷新"：
// 令牌轮换的正确性只有在"旧令牌被用过一次就失效"这个前提下才能验证。
package apptest

import (
	"context"

	"fmt"
	"myblogx/internal/kernel/ctype"
	"sync"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/app"
	"myblogx/internal/auth/domain"
)

// Clock 是可控时钟。
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock 用固定时间点构造。
func NewClock(t time.Time) *Clock { return &Clock{t: t} }

// Now 返回当前时间。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance 推进时间。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Sessions 是内存会话仓储。
type Sessions struct {
	mu       sync.Mutex
	byID     map[domain.SessionID]domain.Session
	nextID   domain.SessionID
	rotates  int
	revokes  int
	clock    *Clock
	failNext error
}

// NewSessions 构造空仓储。
func NewSessions(clock *Clock) *Sessions {
	return &Sessions{byID: map[domain.SessionID]domain.Session{}, clock: clock}
}

// Rotates 返回轮换次数。
func (s *Sessions) Rotates() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rotates
}

// Revokes 返回吊销次数。
func (s *Sessions) Revokes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revokes
}

// Count 返回会话总数（含已吊销）。
func (s *Sessions) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

// FailNext 让下一次写操作失败。
func (s *Sessions) FailNext(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = err
}

func (s *Sessions) takeErr() error {
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return err
	}
	return nil
}

// Create 实现 domain.SessionRepository。
func (s *Sessions) Create(_ context.Context, v *domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.takeErr(); err != nil {
		return err
	}
	s.nextID = ctype.NewID(s.nextID.Uint64() + 1)
	v.ID = s.nextID
	// 与 MySQL 适配器一致：创建时间由仓储落库时补齐，调用方不必自己填。
	// 聚合"首次登录时间"要靠它，替身漏填会让所有设备的首登时间变成零值。
	if v.CreatedAt.IsZero() {
		v.CreatedAt = s.clock.Now()
	}
	s.byID[v.ID] = *v
	return nil
}

// FindByRefreshHash 实现 domain.SessionRepository。
func (s *Sessions) FindByRefreshHash(_ context.Context, hash string) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.byID {
		if v.RefreshTokenHash == hash && v.RevokedAt == nil {
			return v, nil
		}
	}
	return domain.Session{}, apperr.New(apperr.CodeAuthRefreshInvalid)
}

// FindActive 实现 domain.SessionRepository。
func (s *Sessions) FindActive(_ context.Context, id domain.SessionID, userID uint64) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.byID[id]
	if !ok || v.UserID.Uint64() != userID || !v.IsActive(s.clock.Now()) {
		return domain.Session{}, apperr.New(apperr.CodeUserSessionExpired)
	}
	return v, nil
}

// Rotate 实现 domain.SessionRepository。
func (s *Sessions) Rotate(_ context.Context, id domain.SessionID, newHash string, expiresAt time.Time, meta domain.Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.takeErr(); err != nil {
		return err
	}
	v, ok := s.byID[id]
	if !ok {
		return apperr.New(apperr.CodeAuthRefreshInvalid)
	}
	v.RefreshTokenHash = newHash
	v.ExpiresAt = expiresAt
	v.IP = meta.IP
	v.Addr = meta.Addr
	v.UA = meta.UA
	s.byID[id] = v
	s.rotates++
	return nil
}

// Revoke 实现 domain.SessionRepository。
func (s *Sessions) Revoke(_ context.Context, id domain.SessionID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.byID[id]
	if !ok {
		return nil
	}
	if v.RevokedAt == nil {
		v.RevokedAt = &at
		s.revokes++
	}
	s.byID[id] = v
	return nil
}

// RevokeAllForUser 实现 domain.SessionRepository。
func (s *Sessions) RevokeAllForUser(_ context.Context, userID uint64, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, v := range s.byID {
		if v.UserID.Uint64() != userID || v.RevokedAt != nil {
			continue
		}
		v.RevokedAt = &at
		s.byID[id] = v
		s.revokes++
	}
	return nil
}

// Touch 实现 domain.SessionRepository。
func (s *Sessions) Touch(_ context.Context, id domain.SessionID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.byID[id]; ok {
		v.LastSeenAt = &at
		s.byID[id] = v
	}
	return nil
}

// ListActive 实现 domain.SessionRepository。
func (s *Sessions) ListActive(_ context.Context, userID uint64) ([]domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Session, 0, len(s.byID))
	for _, v := range s.byID {
		if v.UserID.Uint64() == userID && v.IsActive(s.clock.Now()) {
			out = append(out, v)
		}
	}
	return out, nil
}

// ------------------------------------------------------------------
// 令牌与加密
// ------------------------------------------------------------------

// Issuer 是假 JWT：令牌是可读字符串，便于断言。
type Issuer struct {
	mu     sync.Mutex
	issued []string
	fail   error
}

// NewIssuer 构造。
func NewIssuer() *Issuer { return &Issuer{} }

// FailWith 让后续签发失败。
func (i *Issuer) FailWith(err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.fail = err
}

// Issue 实现 domain.TokenIssuer。
func (i *Issuer) Issue(c domain.Claims) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.fail != nil {
		return "", i.fail
	}
	token := fmt.Sprintf("access:%s:%s:%d", c.UserID, c.SessionID, c.TokenVersion)
	i.issued = append(i.issued, token)
	return token, nil
}

// Parse 实现 domain.TokenIssuer。
func (i *Issuer) Parse(token string) (domain.Claims, error) {
	var c domain.Claims
	// 令牌是纯数字格式，这里先用数值变量接，再转成 ID 语义。
	var userID, sessionID uint64
	if _, err := fmt.Sscanf(token, "access:%d:%d:%d", &userID, &sessionID, &c.TokenVersion); err != nil {
		return domain.Claims{}, fmt.Errorf("非法令牌: %w", err)
	}
	c.UserID = ctype.NewID(userID)
	c.SessionID = ctype.NewID(sessionID)
	return c, nil
}

// Secrets 是递增序列的假随机串生成器。
type Secrets struct {
	mu   sync.Mutex
	next int
	fail error
}

// NewSecrets 构造。
func NewSecrets() *Secrets { return &Secrets{} }

// FailWith 让后续生成失败。
func (s *Secrets) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// NewRefreshToken 实现 domain.SecretGenerator。
func (s *Secrets) NewRefreshToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	s.next++
	return fmt.Sprintf("refresh-%d", s.next), nil
}

// Hasher 是前缀哈希（可断言但不可逆）。
type Hasher struct{ Prefix string }

// SHA256 实现 domain.Hasher。
func (h Hasher) SHA256(v string) string { return h.Prefix + v }

// Blacklist 是内存黑名单。
type Blacklist struct {
	mu   sync.Mutex
	keys map[string]struct{}
	fail error
}

// NewBlacklist 构造。
func NewBlacklist() *Blacklist { return &Blacklist{keys: map[string]struct{}{}} }

// FailWith 让查询失败。
func (b *Blacklist) FailWith(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fail = err
}

// Add 实现 domain.TokenBlacklist。
func (b *Blacklist) Add(_ context.Context, token string, _ time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.keys[token] = struct{}{}
	return nil
}

// Contains 实现 domain.TokenBlacklist。
func (b *Blacklist) Contains(_ context.Context, token string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail != nil {
		return false, b.fail
	}
	_, ok := b.keys[token]
	return ok, nil
}

// Size 返回黑名单条目数。
func (b *Blacklist) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.keys)
}

// Guard 是可控的登录限流器。
type Guard struct {
	mu       sync.Mutex
	blocked  bool
	failures int
	resets   int
	err      error
}

// NewGuard 构造。
func NewGuard() *Guard { return &Guard{} }

// Block 让后续检查一律拒绝。
func (g *Guard) Block(v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blocked = v
}

// FailWith 让检查报错（用于验证"限流器故障时放行"）。
func (g *Guard) FailWith(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.err = err
}

// Failures 返回记录的失败次数。
func (g *Guard) Failures() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.failures
}

// Resets 返回清零次数。
func (g *Guard) Resets() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.resets
}

// Allowed 实现 domain.LoginGuard。
func (g *Guard) Allowed(_ context.Context, _, _ string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return false, g.err
	}
	return !g.blocked, nil
}

// RecordFailure 实现 domain.LoginGuard。
func (g *Guard) RecordFailure(_ context.Context, _, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failures++
	return nil
}

// Reset 实现 domain.LoginGuard。
func (g *Guard) Reset(_ context.Context, _, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resets++
	return nil
}

// Credentials 是内存账号表。
type Credentials struct {
	mu        sync.Mutex
	byName    map[string]app.Account
	byID      map[uint64]app.Account
	passwd    map[string]string
	failNext  error
	staleNext *staleRead
}

// staleRead 是一次"读到旧值"的读取。
type staleRead struct {
	userID  ctype.ID
	version uint32
}

// NewCredentials 构造。
func NewCredentials() *Credentials {
	return &Credentials{
		byName: map[string]app.Account{},
		byID:   map[uint64]app.Account{},
		passwd: map[string]string{},
	}
}

// Seed 塞入一个账号。
func (c *Credentials) Seed(a app.Account, plain string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byName[a.Username] = a
	c.byID[a.ID.Uint64()] = a
	c.passwd[a.Username] = plain
}

// VerifyPassword 实现 app.Credentials。
func (c *Credentials) VerifyPassword(_ context.Context, identifier, plain string) (app.Account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.byName[identifier]
	if !ok || c.passwd[identifier] != plain {
		return app.Account{}, apperr.New(apperr.CodeUserInvalidCredential)
	}
	return a, nil
}

// FindAccount 实现 app.Credentials。
func (c *Credentials) FindAccount(_ context.Context, userID uint64) (app.Account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.failNext; err != nil {
		c.failNext = nil
		return app.Account{}, err
	}
	a, ok := c.byID[userID]
	if !ok {
		return app.Account{}, apperr.New(apperr.CodeUserTokenInvalid)
	}
	if s := c.staleNext; s != nil && s.userID.Uint64() == userID {
		c.staleNext = nil
		a.TokenVersion = s.version
	}
	return a, nil
}

// FailFindAccount 让下一次查账号失败。
func (c *Credentials) FailFindAccount(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failNext = err
}

// StaleFindAccount 让下一次查指定账号返回旧版本。
//
// 用来表达"读到旧值"这一类存储缺陷（例如查询缓存命中过期条目）：
// 仓储本身的数据是对的，但这一次读返回的是陈旧副本。
func (c *Credentials) StaleFindAccount(userID ctype.ID, version uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.staleNext = &staleRead{userID: userID, version: version}
}

// ------------------------------------------------------------------
// 装配
// ------------------------------------------------------------------

// Repos 把全部替身聚成一套，并提供 Service 构造。
type Repos struct {
	sessions  *Sessions
	issuer    *Issuer
	secrets   *Secrets
	hasher    Hasher
	blacklist *Blacklist
	guard     *Guard
	creds     *Credentials
	clock     *Clock
	opts      app.Options
}

// NewRepos 构造一套全空的替身。
func NewRepos(t time.Time) *Repos {
	clock := NewClock(t)
	return &Repos{
		sessions:  NewSessions(clock),
		issuer:    NewIssuer(),
		secrets:   NewSecrets(),
		hasher:    Hasher{Prefix: "h:"},
		blacklist: NewBlacklist(),
		guard:     NewGuard(),
		creds:     NewCredentials(),
		clock:     clock,
	}
}

// Sessions 返回会话仓储。
func (r *Repos) Sessions() *Sessions { return r.sessions }

// Issuer 返回令牌签发器。
func (r *Repos) Issuer() *Issuer { return r.issuer }

// Secrets 返回随机串生成器。
func (r *Repos) Secrets() *Secrets { return r.secrets }

// Blacklist 返回黑名单。
func (r *Repos) Blacklist() *Blacklist { return r.blacklist }

// Guard 返回限流器。
func (r *Repos) Guard() *Guard { return r.guard }

// Credentials 返回账号表。
func (r *Repos) Credentials() *Credentials { return r.creds }

// Clock 返回时钟。
func (r *Repos) Clock() *Clock { return r.clock }

// WithTTL 覆盖令牌有效期。
func (r *Repos) WithTTL(access, refresh time.Duration) *Repos {
	r.opts = app.Options{AccessTTL: access, RefreshTTL: refresh}
	return r
}

// Service 组装出 app.Service。
func (r *Repos) Service() *app.Service {
	return app.New(
		r.sessions,
		r.issuer,
		r.secrets,
		r.hasher,
		r.blacklist,
		r.guard,
		r.creds,
		r.clock,
		nil,
		r.opts,
	)
}
