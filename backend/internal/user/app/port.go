// Package app 是用户模块的用例层：事务边界、跨模块能力、编排。
//
// 这里不出现 *gorm.DB、不出现 gin。所有外部依赖都是本文件里的小接口，
// 好处是测试可以全部用内存 fake，不需要起库。
package app

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/domain"
)

// Clock 是时间来源（领域层已定义，这里别名一次方便调用）。
type Clock = domain.Clock

// PasswordHasher 负责密码的哈希与校验。
//
// 两者都放在本模块：认证模块（auth）只能拿到「验过了/没验过」的结论，
// 永远拿不到密码哈希本身——这是密码不出用户模块边界的唯一保障。
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(hash, plain string) bool
}

// DetailEnricher 是详情页的"增强读"能力：头像图片 ID 与偏好标签标题。
//
// 这两件事都属于别的模块（media / article），且都只影响展示、不影响正确性，
// 所以合成一个可选端口——查不到就降级，不让详情接口失败。
type DetailEnricher interface {
	// AvatarIDByURL 由头像 URL 反查图片表 ID。
	AvatarIDByURL(ctx context.Context, url string) (ctype.ID, bool, error)
	// AvatarURLByID 由图片 ID 取头像 URL（前端传的是图片 ID）。
	AvatarURLByID(ctx context.Context, id ctype.ID) (string, error)
	// TagTitles 批量取标签标题。
	TagTitles(ctx context.Context, ids []ctype.ID) (map[ctype.ID]string, error)
}

// Logger 是窄日志接口，避免整个用例层依赖 logrus。
type Logger interface {
	Errorf(format string, args ...any)
}

// UsernameAllocator 分配系统自动用户名（来自 Redis 递增序列）。
//
// 邮箱注册 / 第三方登录首次建档时没有用户输入的用户名，只能用系统分配。
// 做成端口而不是直接用 redis_user，是为了让用例层测试不必起 Redis。
type UsernameAllocator interface {
	Next() (string, error)
}

// RelationReader 读当前用户与目标用户的关注关系。
//
// 关系数据在 social 模块，用户模块只消费结论（0 无 / 1 关注 …），
// 不 import 对方的表。
type RelationReader interface {
	Relation(ctx context.Context, viewer, target domain.UserID) (int8, error)
}

// HomeViewRecorder 记录一次主页浏览（按天去重），返回本次是否计入。
type HomeViewRecorder interface {
	Record(ctx context.Context, target, viewer domain.UserID) (bool, error)
}

// SessionRevoker 让某用户全部会话失效（管理员改角色/状态时）。
type SessionRevoker interface {
	RevokeAll(ctx context.Context, userID ctype.ID) error
}

// SessionRevokerFunc 让函数实现 SessionRevoker。
//
// 用来打破装配期的循环依赖：user 服务先建、auth 服务后建，
// 用一个闭包把"后建的 auth 服务"延迟绑定进来。
type SessionRevokerFunc func(ctx context.Context, userID ctype.ID) error

// RevokeAll 实现 SessionRevoker。
func (f SessionRevokerFunc) RevokeAll(ctx context.Context, userID ctype.ID) error {
	return f(ctx, userID)
}

// LastLogin 是列表页展示的最后登录信息。
type LastLogin struct {
	IP   string
	Addr string
	At   time.Time
}

// LastLoginReader 批量读最后登录信息。
type LastLoginReader interface {
	Latest(ctx context.Context, ids []uint64) (map[uint64]LastLogin, error)
}

// TagValidator 校验偏好标签都存在且启用。
type TagValidator interface {
	Validate(ctx context.Context, ids []ctype.ID) error
}

// Integrations 是用户模块的可选外部能力集合。
//
// 合成一个结构而不是六个 WithXxx：它们都是"详情页/管理端"的锦上添花能力，
// 装配时一次性给齐或全不给，拆开只会让 New 的参数列表继续膨胀。
type Integrations struct {
	Relation  RelationReader
	HomeView  HomeViewRecorder
	Sessions  SessionRevoker
	LastLogin LastLoginReader
	Tags      TagValidator
}

// WithIntegrations 注入可选外部能力。
func WithIntegrations(i Integrations) Option {
	return func(s *Service) {
		s.relation = i.Relation
		s.homeView = i.HomeView
		s.sessions = i.Sessions
		s.lastLogin = i.LastLogin
		s.tags = i.Tags
	}
}

// UserInfoPort 是本模块提供给其他模块的只读能力。
//
// 定义在**提供方**（本包），消费方只依赖这个接口：
//   - article 渲染作者信息 → LoadDisplays
//   - 任何模块要判断"这个用户还能不能发内容" → EnsureUsable
//
// 之所以不直接暴露 domain.User：那会把密码哈希、邮箱一起递出去。
type UserInfoPort interface {
	// LoadDisplays 批量读取展示快照；不存在的 ID 直接不出现在结果里。
	LoadDisplays(ctx context.Context, ids []domain.UserID) (map[domain.UserID]domain.Display, error)
	// EnsureUsable 校验用户处于可写状态（存在且未被禁用/封禁）。
	EnsureUsable(ctx context.Context, id domain.UserID) error
}

// CredentialPort 是给认证模块（auth）用的凭证能力。
//
// 与 UserInfoPort 分开：那个只给展示用的公开信息，这个能拿到完整 User
// （含密码哈希），只有认证场景才该用。
//
// 刻意不提供「按 ID 查密码哈希」的方法：auth 只需要验密码，不需要读哈希。
type CredentialPort interface {
	// VerifyPassword 用「用户名或邮箱 + 明文密码」校验，成功返回用户。
	//
	// 用户名不存在、没设密码、密码错误三种情况返回同一个错误码
	// （CodeUserInvalidCredential），否则攻击者可以据此枚举账号。
	VerifyPassword(ctx context.Context, identifier, plain string) (domain.User, error)
	// FindForAuth 按 ID 查用户，供令牌校验时判断状态与版本号。
	FindForAuth(ctx context.Context, id domain.UserID) (domain.User, error)
}

// Service 是用户模块的应用服务。
//
// 一个 Service 同时实现 UserInfoPort——读能力和写用例共享同一套仓储，
// 没必要拆成两个结构体再互相注入。
type Service struct {
	users         domain.Repository
	settings      domain.SettingsRepository
	stats         domain.StatsRepository
	snapshots     domain.SnapshotSyncer
	enricher      DetailEnricher
	clock         Clock
	hasher        PasswordHasher
	usernameAlloc UsernameAllocator
	logger        Logger

	relation  RelationReader
	homeView  HomeViewRecorder
	sessions  SessionRevoker
	lastLogin LastLoginReader
	tags      TagValidator
}

// Option 是可选的装配项。
//
// 用可变参数而不是往 New 再加一个必填参数：已有的测试与调用方不需要感知
// 新增能力（账号生命周期相关的装配只有生产路径才需要）。
type Option func(*Service)

// WithUsernameAllocator 注入自动用户名分配器。
func WithUsernameAllocator(a UsernameAllocator) Option {
	return func(s *Service) { s.usernameAlloc = a }
}

// New 构造服务。
//
// 用显式参数而不是依赖结构体：archcheck 的 R2 禁止 `type Options struct`，
// 那条规则的本意是防止再造一个"什么都往里塞"的袋子。显式参数逼着调用方
// 写清每一个依赖，代价只是构造行有点长。
//
// 缺关键依赖时直接 panic：这类错误应该在启动期暴露，而不是运行到半夜某个
// 请求才 nil panic。
func New(
	users domain.Repository,
	settings domain.SettingsRepository,
	stats domain.StatsRepository,
	snapshots domain.SnapshotSyncer,
	enricher DetailEnricher,
	clock Clock,
	hasher PasswordHasher,
	logger Logger,
	extras ...Option,
) *Service {
	if users == nil {
		panic("user/app: 用户仓储未注入")
	}
	if settings == nil {
		panic("user/app: 偏好仓储未注入")
	}
	if stats == nil {
		panic("user/app: 统计仓储未注入")
	}
	if clock == nil {
		panic("user/app: 时钟未注入")
	}
	svc := &Service{
		users:     users,
		settings:  settings,
		stats:     stats,
		snapshots: snapshots,
		enricher:  enricher,
		clock:     clock,
		hasher:    hasher,
		logger:    logger,
	}
	for _, opt := range extras {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// 编译期断言：Service 同时满足两个对外端口。
var (
	_ UserInfoPort   = (*Service)(nil)
	_ CredentialPort = (*Service)(nil)
)

// LoadDisplays 实现 UserInfoPort。
func (s *Service) LoadDisplays(ctx context.Context, ids []domain.UserID) (map[domain.UserID]domain.Display, error) {
	// 仓储层是 DisplayReader 的实现，这里做一次类型断言而不是往 Options 再加一个字段：
	// 读快照是"仓储顺带具备"的能力，不是所有仓储实现都必须支持。
	reader, ok := s.users.(domain.DisplayReader)
	if !ok {
		return map[domain.UserID]domain.Display{}, nil
	}
	return reader.LoadDisplays(ctx, ids)
}

// EnsureUsable 实现 UserInfoPort。
func (s *Service) EnsureUsable(ctx context.Context, id domain.UserID) error {
	if id.IsZero() {
		return notFound()
	}
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return domain.CanLogin(u)
}
