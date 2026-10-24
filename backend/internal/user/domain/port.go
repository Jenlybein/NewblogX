package domain

import (
	"context"
	"time"
)

// Clock 提供当前时间。
//
// 领域层不直接调 time.Now：否则"用户名冷却 30 天""注册年数"这类规则只能靠
// 改系统时间测试，要么就干脆不测。archcheck 的 R4 会把 direct time.Now 标出来。
type Clock interface {
	Now() time.Time
}

// Query 是用户列表的查询条件。
type Query struct {
	Keyword string
	Status  *Status
	Role    *Role
	Page    int
	Size    int
}

// Normalize 把分页参数收敛到安全范围。
func (q Query) Normalize() Query {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Size <= 0 {
		q.Size = 20
	}
	if q.Size > 100 {
		q.Size = 100
	}
	return q
}

// Offset 返回 SQL 偏移量。
func (q Query) Offset() int { return (q.Page - 1) * q.Size }

// Page 是一页用户。
type Page struct {
	Items []User
	Total int64
}

// StatsDelta 是统计字段的增量。
//
// 用增量而不是"设成多少"：并发下两个请求各 +1，后写的会把先写的覆盖掉。
type StatsDelta struct {
	View           int
	Fans           int
	Follow         int
	ArticleVisited int
	Article        int
}

// IsZero 判断是否没有任何增量（省掉一次无意义的 UPDATE）。
func (d StatsDelta) IsZero() bool {
	return d.View == 0 && d.Fans == 0 && d.Follow == 0 && d.ArticleVisited == 0 && d.Article == 0
}

// AdminPatch 是管理员修改用户信息的补丁。
//
// 全指针：PATCH 语义下"没传"和"传空"是两件事。角色/状态单独给，
// 因为它们变化时要连带作废该用户全部登录态。
type AdminPatch struct {
	Username  *string
	Nickname  *string
	Abstract  *string
	AvatarURL *string
	Role      *Role
	Status    *Status
}

// Repository 是用户聚合的持久化端口。
//
// 所有方法都接收 ctx；错误一律返回 apperr 错误码，不返回 gorm 的原始错误——
// 上层不该知道"哪张表"。
type Repository interface {
	// FindByID 查用户；不存在时返回 apperr.CodeUserNotFound。
	FindByID(ctx context.Context, id UserID) (User, error)
	// FindByUsername 按用户名查；不存在时返回 apperr.CodeUserNotFound。
	FindByUsername(ctx context.Context, username string) (User, error)
	// FindByEmail 按邮箱查；不存在时返回 apperr.CodeUserNotFound。
	FindByEmail(ctx context.Context, email string) (User, error)
	// FindByOpenID 按第三方 openid 查；不存在时返回 apperr.CodeUserNotFound。
	FindByOpenID(ctx context.Context, openID string) (User, error)
	// ExistsUsername 判断用户名是否被占用（exclude 用于排除自己）。
	ExistsUsername(ctx context.Context, username string, exclude UserID) (bool, error)
	// ExistsEmail 判断邮箱是否被占用（exclude 用于排除自己）。
	ExistsEmail(ctx context.Context, email string, exclude UserID) (bool, error)
	// Create 落库并回填 ID。
	Create(ctx context.Context, user *User) error
	// UpdateProfile 只更新资料字段，updatedUsernameAt 非空时一并写冷却时间戳。
	UpdateProfile(ctx context.Context, id UserID, update ProfileUpdate, updatedUsernameAt *time.Time) error
	// UpdateStatus 只更新账号状态。
	UpdateStatus(ctx context.Context, id UserID, status Status) error
	// UpdatePassword 改写密码哈希与令牌版本（版本 +1 让旧令牌立即失效）。
	UpdatePassword(ctx context.Context, id UserID, hash string, tokenVersion uint32) error
	// UpdateEmail 绑定或换绑邮箱。
	UpdateEmail(ctx context.Context, id UserID, email string) error
	// UpdateAdmin 应用管理员补丁（只写传入的列）。
	UpdateAdmin(ctx context.Context, id UserID, patch AdminPatch) error
	// BumpTokenVersion 把令牌版本 +1，让该用户所有旧访问令牌立即失效。
	BumpTokenVersion(ctx context.Context, id UserID) error
	// List 按条件分页查询。
	List(ctx context.Context, q Query) (Page, error)
}

// SettingsRepository 是用户偏好设置的持久化端口。
type SettingsRepository interface {
	// Find 查偏好；不存在时返回默认值而不是错误——老账号可能没有这条记录。
	Find(ctx context.Context, id UserID) (Settings, error)
	// Save 保存偏好。
	Save(ctx context.Context, s Settings) error
	// InitDefaults 幂等建档（已存在则什么都不做）。
	InitDefaults(ctx context.Context, id UserID) error
}

// StatsRepository 是用户统计的持久化端口。
type StatsRepository interface {
	// Find 查统计；不存在时返回零值。
	Find(ctx context.Context, id UserID) (Stats, error)
	// Init 幂等建档。
	Init(ctx context.Context, id UserID) error
	// Add 按增量累加。
	Add(ctx context.Context, id UserID, delta StatsDelta) error
}

// DisplayReader 批量读用户展示快照。
//
// 这个端口的存在就是为了让别的模块不 import 本模块的 adapter：
// 给一批 ID 拿一批昵称头像，一次查询，没有 N+1。
type DisplayReader interface {
	LoadDisplays(ctx context.Context, ids []UserID) (map[UserID]Display, error)
}

// SnapshotSyncer 把昵称/头像/简介同步到其他表里的冗余列。
//
// 现状是评论、关注、收藏、私信、收藏夹等 7 张表都存了一份用户昵称头像。
// 这是有意的反范式（列表页不 join 用户表），代价是改资料时要同步刷新。
// 同步失败只记日志不阻断——资料已经改成功了，回滚反而更糟。
type SnapshotSyncer interface {
	SyncDisplay(ctx context.Context, id UserID) error
}
