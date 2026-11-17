package domain

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
)

// AppealWindow 是申诉窗口。
//
// 7 天：短到能减少"陈年申诉"的运营负担，长到足以让周末才发现自己被禁言的
// 用户还能提交。超过窗口的申诉在提交入口就被拒，不留"已读不回"的悬案。
const AppealWindow = 7 * 24 * time.Hour

// AppealStatus 是申诉状态。
type AppealStatus string

const (
	// AppealPending 待处理。
	AppealPending AppealStatus = "pending"
	// AppealApproved 已通过（事件撤销、处分解除）。
	AppealApproved AppealStatus = "approved"
	// AppealRejected 已驳回（必须填理由，用户可见）。
	AppealRejected AppealStatus = "rejected"
)

// Appeal 是一次申诉。
type Appeal struct {
	ID        ctype.ID
	EventID   ctype.ID
	UserID    ctype.ID
	Reason    string
	Status    AppealStatus
	HandledBy ctype.ID
	HandledAt *time.Time
	Note      string
	CreatedAt time.Time
}

// AppealRepository 是申诉的持久化端口。
type AppealRepository interface {
	// Create 创建一条申诉。同一事件只能申诉一次（唯一键）。
	Create(ctx context.Context, appeal Appeal) (Appeal, error)
	// FindByEvent 按事件取申诉。
	FindByEvent(ctx context.Context, eventID ctype.ID) (Appeal, bool, error)
	// FindByID 按 ID 取申诉。
	FindByID(ctx context.Context, id ctype.ID) (Appeal, bool, error)
	// List 按状态取申诉（空串表示全部）。
	List(ctx context.Context, status AppealStatus, limit int) ([]Appeal, error)
	// Handle 落一次处置结果。
	Handle(ctx context.Context, id, operator ctype.ID, status AppealStatus, note string, at time.Time) error
}

// CanAppeal 判断一条事件现在还能不能申诉。
//
// 三条规则：
//   - 已撤销的事件无需申诉（分数已经退回去了）；
//   - 只有 7 天内的事件可申诉；
//   - 已经申诉过的不能再提（列表由调用方查，这里只判时间与状态）。
func CanAppeal(event Event, now time.Time) bool {
	if event.Status != StatusActive {
		return false
	}
	return !event.CreatedAt.Add(AppealWindow).Before(now)
}
