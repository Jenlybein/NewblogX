// Package apptest 提供用户模块的有状态测试替身。
//
// 不是 gomock：这些 fake 有真实状态（内存 map），能模拟"创建后再查"、
// "同名冲突"、"写失败"这类需要连续调用才能暴露的问题。gomock 只能校验
// "调用了几次、参数是什么"，测不出业务结果。
//
// 注意：本包 import app（为了满足接口），因此 **app 包的测试必须用 package app_test**，
// 否则会形成 app → apptest → app 的循环依赖。
package apptest

import (
	"context"
	"sync"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
)

// ------------------------------------------------------------------
// 时钟与哈希
// ------------------------------------------------------------------

// Clock 是可控时钟。
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock 用固定时间点构造时钟，保证测试可复现。
func NewClock(t time.Time) *Clock { return &Clock{t: t} }

// Now 返回当前时间。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance 把时间往前推，用于测试冷却期。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Hasher 是固定前缀的假哈希器（不引入 bcrypt 的随机盐，才能断言）。
type Hasher struct{ Prefix string }

// Hash 返回 "前缀+明文"。
func (h Hasher) Hash(plain string) (string, error) { return h.Prefix + plain, nil }

// Verify 比较 "前缀+明文"。
func (h Hasher) Verify(hash, plain string) bool { return hash == h.Prefix+plain }

// ------------------------------------------------------------------
// 用户仓储
// ------------------------------------------------------------------

// ProfileCall 记录一次 UpdateProfile 调用。
type ProfileCall struct {
	ID     domain.UserID
	Update domain.ProfileUpdate
	At     *time.Time
}

// Users 是内存用户仓储。
type Users struct {
	mu        sync.Mutex
	byID      map[domain.UserID]domain.User
	nextID    domain.UserID
	profile   []ProfileCall
	failNext  error
	failReads error
}

// NewUsers 构造空仓储。
func NewUsers() *Users {
	return &Users{byID: map[domain.UserID]domain.User{}, nextID: 1000}
}

// FailNext 让下一次写操作失败一次（用于测试失败分支）。
func (u *Users) FailNext(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.failNext = err
}

// FailReads 让后续读操作全部失败。
func (u *Users) FailReads(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.failReads = err
}

// Seed 直接塞入一个用户（绕过业务校验，仅用于准备数据）。
func (u *Users) Seed(user domain.User) domain.User {
	u.mu.Lock()
	defer u.mu.Unlock()
	if user.ID.IsZero() {
		u.nextID++
		user.ID = u.nextID
	}
	u.byID[user.ID] = user
	return user
}

// ProfileCalls 返回 UpdateProfile 的调用记录。
func (u *Users) ProfileCalls() []ProfileCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]ProfileCall, len(u.profile))
	copy(out, u.profile)
	return out
}

func (u *Users) takeErr() error {
	if u.failNext != nil {
		err := u.failNext
		u.failNext = nil
		return err
	}
	return nil
}

// FindByID 实现 domain.Repository。
func (u *Users) FindByID(_ context.Context, id domain.UserID) (domain.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.failReads != nil {
		return domain.User{}, u.failReads
	}
	user, ok := u.byID[id]
	if !ok {
		return domain.User{}, apperr.New(apperr.CodeUserNotFound)
	}
	return user, nil
}

// FindByUsername 实现 domain.Repository。
func (u *Users) FindByUsername(_ context.Context, username string) (domain.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, user := range u.byID {
		if user.Username == username {
			return user, nil
		}
	}
	return domain.User{}, apperr.New(apperr.CodeUserNotFound)
}

// FindByEmail 实现 domain.Repository。
func (u *Users) FindByEmail(_ context.Context, email string) (domain.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, user := range u.byID {
		if user.Email != nil && *user.Email == email {
			return user, nil
		}
	}
	return domain.User{}, apperr.New(apperr.CodeUserNotFound)
}

// FindByOpenID 实现 domain.Repository。
func (u *Users) FindByOpenID(_ context.Context, openID string) (domain.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, user := range u.byID {
		if user.OpenID != nil && *user.OpenID == openID {
			return user, nil
		}
	}
	return domain.User{}, apperr.New(apperr.CodeUserNotFound)
}

// ExistsUsername 实现 domain.Repository。
func (u *Users) ExistsUsername(_ context.Context, username string, exclude domain.UserID) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, user := range u.byID {
		if user.Username == username && user.ID != exclude {
			return true, nil
		}
	}
	return false, nil
}

// ExistsEmail 实现 domain.Repository。
func (u *Users) ExistsEmail(_ context.Context, email string, exclude domain.UserID) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, user := range u.byID {
		if user.Email != nil && *user.Email == email && user.ID != exclude {
			return true, nil
		}
	}
	return false, nil
}

// Create 实现 domain.Repository。
func (u *Users) Create(_ context.Context, user *domain.User) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	u.nextID++
	user.ID = u.nextID
	u.byID[user.ID] = *user
	return nil
}

// UpdateProfile 实现 domain.Repository。
func (u *Users) UpdateProfile(_ context.Context, id domain.UserID, update domain.ProfileUpdate, at *time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	user.ApplyProfileUpdate(update)
	u.byID[id] = user
	u.profile = append(u.profile, ProfileCall{ID: id, Update: update, At: at})
	return nil
}

// UpdateStatus 实现 domain.Repository。
func (u *Users) UpdateStatus(_ context.Context, id domain.UserID, status domain.Status) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	user.Status = status
	u.byID[id] = user
	return nil
}

// UpdatePassword 实现 domain.Repository。
func (u *Users) UpdatePassword(_ context.Context, id domain.UserID, hash string, tokenVersion uint32) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	user.PasswordHash = hash
	user.TokenVersion = tokenVersion
	u.byID[id] = user
	return nil
}

// UpdateEmail 实现 domain.Repository。
func (u *Users) UpdateEmail(_ context.Context, id domain.UserID, email string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	value := email
	user.Email = &value
	u.byID[id] = user
	return nil
}

// UpdateAdmin 实现 domain.Repository。
func (u *Users) UpdateAdmin(_ context.Context, id domain.UserID, patch domain.AdminPatch) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	if patch.Username != nil {
		user.Username = *patch.Username
	}
	if patch.Nickname != nil {
		user.Nickname = *patch.Nickname
	}
	if patch.Abstract != nil {
		user.Abstract = *patch.Abstract
	}
	if patch.AvatarURL != nil {
		user.Avatar = *patch.AvatarURL
	}
	if patch.Role != nil {
		user.Role = *patch.Role
	}
	if patch.Status != nil {
		user.Status = *patch.Status
	}
	u.byID[id] = user
	return nil
}

// BumpTokenVersion 实现 domain.Repository。
func (u *Users) BumpTokenVersion(_ context.Context, id domain.UserID) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.takeErr(); err != nil {
		return err
	}
	user, ok := u.byID[id]
	if !ok {
		return apperr.New(apperr.CodeUserNotFound)
	}
	user.TokenVersion++
	u.byID[id] = user
	return nil
}

// List 实现 domain.Repository。
func (u *Users) List(_ context.Context, q domain.Query) (domain.Page, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	items := make([]domain.User, 0, len(u.byID))
	for _, user := range u.byID {
		if q.Status != nil && user.Status != *q.Status {
			continue
		}
		if q.Role != nil && user.Role != *q.Role {
			continue
		}
		items = append(items, user)
	}
	return domain.Page{Items: items, Total: int64(len(items))}, nil
}

// LoadDisplays 实现 domain.DisplayReader。
func (u *Users) LoadDisplays(_ context.Context, ids []domain.UserID) (map[domain.UserID]domain.Display, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.failReads != nil {
		return nil, u.failReads
	}
	out := make(map[domain.UserID]domain.Display, len(ids))
	for _, id := range ids {
		if user, ok := u.byID[id]; ok {
			out[id] = domain.Display{
				ID:       user.ID,
				Nickname: user.Nickname,
				Avatar:   user.Avatar,
				Abstract: user.Abstract,
				Role:     user.Role,
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------------
// 偏好设置
// ------------------------------------------------------------------

// Settings 是内存偏好仓储。
type Settings struct {
	mu     sync.Mutex
	byUser map[domain.UserID]domain.Settings
	saves  int
}

// NewSettings 构造空仓储。
func NewSettings() *Settings {
	return &Settings{byUser: map[domain.UserID]domain.Settings{}}
}

// Saves 返回调用 Save 的次数。
func (s *Settings) Saves() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

// Find 实现 domain.SettingsRepository；不存在时返回默认值。
func (s *Settings) Find(_ context.Context, id domain.UserID) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.byUser[id]; ok {
		return v, nil
	}
	return domain.DefaultSettings(id), nil
}

// Save 实现 domain.SettingsRepository。
func (s *Settings) Save(_ context.Context, v domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byUser[v.UserID] = v
	s.saves++
	return nil
}

// InitDefaults 实现 domain.SettingsRepository。
func (s *Settings) InitDefaults(_ context.Context, id domain.UserID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byUser[id]; ok {
		return nil
	}
	s.byUser[id] = domain.DefaultSettings(id)
	return nil
}

// ------------------------------------------------------------------
// 统计
// ------------------------------------------------------------------

// Stats 是内存统计仓储。
type Stats struct {
	mu     sync.Mutex
	byUser map[domain.UserID]domain.Stats
	deltas []domain.StatsDelta
}

// NewStats 构造空仓储。
func NewStats() *Stats {
	return &Stats{byUser: map[domain.UserID]domain.Stats{}}
}

// Deltas 返回收到的增量列表。
func (s *Stats) Deltas() []domain.StatsDelta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.StatsDelta, len(s.deltas))
	copy(out, s.deltas)
	return out
}

// Find 实现 domain.StatsRepository。
func (s *Stats) Find(_ context.Context, id domain.UserID) (domain.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.byUser[id]; ok {
		return v, nil
	}
	return domain.ZeroStats(id), nil
}

// Init 实现 domain.StatsRepository。
func (s *Stats) Init(_ context.Context, id domain.UserID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byUser[id]; !ok {
		s.byUser[id] = domain.ZeroStats(id)
	}
	return nil
}

// Add 实现 domain.StatsRepository。
func (s *Stats) Add(_ context.Context, id domain.UserID, delta domain.StatsDelta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.byUser[id]
	if !ok {
		v = domain.ZeroStats(id)
	}
	v.ViewCount += delta.View
	v.FansCount += delta.Fans
	v.FollowCount += delta.Follow
	v.ArticleVisitedCount += delta.ArticleVisited
	v.ArticleCount += delta.Article
	s.byUser[id] = v
	s.deltas = append(s.deltas, delta)
	return nil
}

// ------------------------------------------------------------------
// 快照同步
// ------------------------------------------------------------------

// Snapshots 是假快照同步器。
type Snapshots struct {
	mu    sync.Mutex
	calls []domain.UserID
	fail  error
}

// NewSnapshots 构造。
func NewSnapshots() *Snapshots { return &Snapshots{} }

// FailWith 让后续同步全部失败。
func (s *Snapshots) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// Calls 返回被同步过的用户 ID。
func (s *Snapshots) Calls() []domain.UserID {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.UserID, len(s.calls))
	copy(out, s.calls)
	return out
}

// SyncDisplay 实现 domain.SnapshotSyncer。
func (s *Snapshots) SyncDisplay(_ context.Context, id domain.UserID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.calls = append(s.calls, id)
	return nil
}

// ------------------------------------------------------------------
// 标签与头像
// ------------------------------------------------------------------

// Enricher 同时实现 app.DetailEnricher：头像反查 + 标签标题。
//
// 用指针接收，测试可以先 NewRepos 再往里塞数据。
type Enricher struct {
	TagTitleMap map[uint64]string
	AvatarIDMap map[string]uint64
}

// NewEnricher 构造空增强器。
func NewEnricher() *Enricher {
	return &Enricher{TagTitleMap: map[uint64]string{}, AvatarIDMap: map[string]uint64{}}
}

// AvatarIDByURL 实现 app.DetailEnricher。
func (e *Enricher) AvatarIDByURL(_ context.Context, url string) (uint64, bool, error) {
	id, ok := e.AvatarIDMap[url]
	return id, ok, nil
}

// AvatarURLByID 实现 app.DetailEnricher。
func (e *Enricher) AvatarURLByID(_ context.Context, id uint64) (string, error) {
	for url, got := range e.AvatarIDMap {
		if got == id {
			return url, nil
		}
	}
	return "", nil
}

// TagTitles 实现 app.DetailEnricher。
func (e *Enricher) TagTitles(_ context.Context, ids []uint64) (map[uint64]string, error) {
	out := make(map[uint64]string, len(ids))
	for _, id := range ids {
		if title, ok := e.TagTitleMap[id]; ok {
			out[id] = title
		}
	}
	return out, nil
}

// ------------------------------------------------------------------
// 装配
// ------------------------------------------------------------------

// Repos 把全部 fake 聚成一套 app.Options。
type Repos struct {
	users     *Users
	settings  *Settings
	stats     *Stats
	snapshots *Snapshots
	clock     *Clock
	hasher    Hasher
	enricher  *Enricher
}

// NewRepos 构造一套全空的替身。
func NewRepos(t time.Time) *Repos {
	return &Repos{
		users:     NewUsers(),
		settings:  NewSettings(),
		stats:     NewStats(),
		snapshots: NewSnapshots(),
		clock:     NewClock(t),
		hasher:    Hasher{Prefix: "hash:"},
		enricher:  NewEnricher(),
	}
}

// Users 返回用户仓储 fake（可直接 Seed / 注入故障）。
func (r *Repos) Users() *Users { return r.users }

// Settings 返回偏好仓储 fake。
func (r *Repos) Settings() *Settings { return r.settings }

// Stats 返回统计仓储 fake。
func (r *Repos) Stats() *Stats { return r.stats }

// Snapshots 返回快照同步 fake。
func (r *Repos) Snapshots() *Snapshots { return r.snapshots }

// Clock 返回可控时钟。
func (r *Repos) Clock() *Clock { return r.clock }

// Enricher 返回详情增强 fake（标签标题 / 头像 ID）。
func (r *Repos) Enricher() *Enricher { return r.enricher }

// Service 组装出一个 app.Service。
func (r *Repos) Service() *app.Service {
	return app.New(
		r.users,
		r.settings,
		r.stats,
		r.snapshots,
		r.enricher,
		r.clock,
		r.hasher,
		nil, // logger：测试里不需要
	)
}
