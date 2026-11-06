// Package redis 是私信模块的 Redis 适配器：发送限流与 WS 票据。
package redis

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_chat"
	"myblogx/internal/platform/redis/redis_ws_ticket"
)

// Limiter 实现 domain.Limiter。
//
// 限流口径（ADR-0026）：同一发件人对同一收件人 1 条 / N 秒、M 条 / 自然日。
// N 与 M **每次发送现读**生效值模块给出的闭包（后台「站点 → 私信」可改），
// 不重启即生效；默认值与合法范围只在生效值模块声明一次。
type Limiter struct {
	deps redis_service.Options
	now  func() time.Time
	// limits 返回 (窗口秒数, 日额度)；注入函数而不是快照，保证"现读"。
	limits func() (int, int)
}

var _ domain.Limiter = (*Limiter)(nil)

// NewLimiter 构造限流器；limits 为 nil 时回落到生效值模块的默认值。
func NewLimiter(deps redis_service.Options, limits func() (int, int)) *Limiter {
	if limits == nil {
		limits = config.NewEffectiveConfig(nil).ChatRateFunc()
	}
	return &Limiter{deps: deps, now: time.Now, limits: limits}
}

// Reserve 预占发送额度。
//
// 失败关闭：Redis 异常一律返回错误，不放行。落库失败 / 幂等命中时由用例层
// 调用 Rollback 释放预占，避免失败请求占掉额度。
func (l *Limiter) Reserve(_ context.Context, senderID, receiverID ctype.ID, _ int8) (domain.Reservation, error) {
	windowSec, dailyMax := l.limits()
	reservation, limitedBy, err := redis_chat.ReserveChatPairRate(l.deps, senderID, receiverID, l.now(), windowSec, dailyMax)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		if limitedBy == "daily" {
			return nil, apperr.New(apperr.CodeChatDailyLimit)
		}
		return nil, apperr.New(apperr.CodeChatTooFrequent)
	}
	return &chatReservation{inner: reservation, deps: l.deps}, nil
}

// chatReservation 把 Redis 适配器的预占适配成 domain.Reservation。
type chatReservation struct {
	inner *redis_chat.PairReservation
	deps  redis_service.Options
}

func (r *chatReservation) Commit() error {
	// 额度已由 Lua 脚本原子写入，提交阶段无需再动；保留接口是为与发件箱式
	// "预占 → 落库 → 提交"的语义对齐（ADR-0022）。
	return nil
}

func (r *chatReservation) Rollback() error {
	if r == nil || r.inner == nil {
		return nil
	}
	return r.inner.Release(r.deps)
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
		UserID:    ctype.NewID(userID),
		SessionID: ctype.NewID(sessionID),
	}, ttl)
}

// Consume 一次性消费票据。
func (t *Tickets) Consume(_ context.Context, ticket string) (uint64, uint64, bool, error) {
	payload, err := redis_ws_ticket.Consume(t.deps, ticket)
	if err != nil {
		return 0, 0, false, err
	}
	return payload.UserID.Uint64(), payload.SessionID.Uint64(), true, nil
}
