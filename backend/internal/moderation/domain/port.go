package domain

import (
	"context"
	"time"
)

// Repository 是违规事件账本的写读端口。
//
// 账本只追加：撤销是改状态，不是删行 —— 否则"为什么这个账号曾经被封过"
// 这件事就查不出来了。
type Repository interface {
	// Append 追加一条事件。同 (ref_type, ref_id, user_id) 重复写入返回 inserted=false。
	//
	// 幂等由唯一键保证而不是"先查再写"：两个请求同时判定同一张图违规时，
	// 先查再写会双双通过，结果是同一件事被扣两次分。
	Append(ctx context.Context, event Event) (Event, bool, error)
	// ListByUser 按时间倒序取某用户的事件。
	ListByUser(ctx context.Context, userID uint64, limit int) ([]Event, error)
	// FindByID 取单条事件。
	FindByID(ctx context.Context, id uint64) (Event, bool, error)
	// Revoke 撤销一条事件（申诉通过或管理员纠错）。
	Revoke(ctx context.Context, id, operator uint64, note string, at time.Time) error
}

// Account 是处罚端口：把档位落到账号上。
//
// 单独抽出来是因为"禁言"这件事的语义归用户模块（mute_until、到期惰性判定、
// 每小时对账都在那边）。违规计分只决定"该不该禁、禁多久"。
type Account interface {
	// IsAdmin 判断是否管理员账号：管理员不记分、不被封禁。
	IsAdmin(ctx context.Context, userID uint64) (bool, error)
	// IsMuted 判断当前是否处在禁言期（用于"封禁期内再违规升一档"）。
	IsMuted(ctx context.Context, userID uint64) (bool, error)
	// Mute 禁言指定天数。
	Mute(ctx context.Context, userID uint64, days int) error
	// Unmute 解除禁言（申诉通过后把后果退回去）。
	Unmute(ctx context.Context, userID uint64) error
}

// Notifier 是站内通知端口。
type Notifier interface {
	NotifySystem(ctx context.Context, receiver uint64, content, linkTitle, linkHref string) error
}

// Clock 提供当前时间（规则层不自己取时间，否则测不了衰减）。
type Clock interface {
	Now() time.Time
}

// Logger 是这一层需要的最小日志面。
type Logger interface {
	Warnf(format string, args ...any)
}
