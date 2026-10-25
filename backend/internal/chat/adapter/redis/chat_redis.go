// Package redis 是私信模块的 Redis 适配器：发送限流与 WS 票据。
package redis

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_chat"
	"myblogx/internal/platform/redis/redis_ws_ticket"
)

// Limiter 实现 domain.Limiter。
type Limiter struct {
	deps redis_service.Options
}

var _ domain.Limiter = (*Limiter)(nil)

// NewLimiter 构造限流器。
func NewLimiter(deps redis_service.Options) *Limiter { return &Limiter{deps: deps} }

// Reserve 预占发送额度。
func (l *Limiter) Reserve(_ context.Context, senderID, receiverID ctype.ID, relation int8) (domain.Reservation, error) {
	now := time.Now()
	sessionID := domain.BuildSessionID(senderID, receiverID)

	minute, limitedBy, err := redis_chat.ReserveChatMinuteRate(l.deps, senderID, sessionID, now)
	if err != nil {
		return nil, err
	}
	if minute == nil {
		if limitedBy == "session" {
			return nil, apperr.New(apperr.CodeChatSessionTooFast)
		}
		return nil, apperr.New(apperr.CodeChatTooFrequent)
	}

	res := &reservation{
		minute:     minute,
		senderID:   senderID,
		receiverID: receiverID,
		now:        now,
		deps:       l.deps,
	}

	// 自聊与好友不受自然周配额限制。
	if senderID == receiverID || relation == 4 {
		return res, nil
	}

	limit := 3
	if relation == 1 {
		limit = 1
	}
	week, allowed, err := redis_chat.ReserveChatWeekQuota(l.deps, senderID, receiverID, limit, now)
	if err != nil {
		_ = res.Rollback()
		return nil, err
	}
	if !allowed {
		_ = res.Rollback()
		if relation == 1 {
			return nil, apperr.New(apperr.CodeChatWeekStranger)
		}
		return nil, apperr.New(apperr.CodeChatWeekLimit)
	}
	res.week = week
	res.resetOnCommit = relation != 1
	return res, nil
}

// reservation 实现 domain.Reservation。
type reservation struct {
	minute        *redis_chat.MinuteReservation
	week          *redis_chat.WeekQuotaReservation
	resetOnCommit bool
	senderID      ctype.ID
	receiverID    ctype.ID
	now           time.Time
	deps          redis_service.Options
}

// Commit 发送成功后提交；单向关系顺手清掉对向周配额（表示对方已回复）。
func (r *reservation) Commit() error {
	if r == nil || !r.resetOnCommit {
		return nil
	}
	return redis_chat.ResetChatWeekQuota(r.deps, r.receiverID, r.senderID, r.now)
}

// Rollback 发送失败时释放预占。
func (r *reservation) Rollback() error {
	if r == nil {
		return nil
	}
	if r.week != nil {
		if err := r.week.Release(r.deps); err != nil {
			return err
		}
	}
	if r.minute != nil {
		return r.minute.Release(r.deps)
	}
	return nil
}

// Tickets 实现 WS 票据存取。
type Tickets struct {
	deps redis_service.Options
}

// NewTickets 构造票据存储。
func NewTickets(deps redis_service.Options) *Tickets { return &Tickets{deps: deps} }

// Store 存票据。
func (t *Tickets) Store(_ context.Context, ticket string, userID, sessionID uint64, ttl time.Duration) error {
	return redis_ws_ticket.Store(t.deps, ticket, redis_ws_ticket.TicketPayload{
		UserID:    ctype.ID(userID),
		SessionID: ctype.ID(sessionID),
	}, ttl)
}

// Consume 一次性消费票据。
func (t *Tickets) Consume(_ context.Context, ticket string) (uint64, uint64, bool, error) {
	payload, err := redis_ws_ticket.Consume(t.deps, ticket)
	if err != nil {
		return 0, 0, false, err
	}
	return uint64(payload.UserID), uint64(payload.SessionID), true, nil
}
