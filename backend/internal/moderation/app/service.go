// Package app 是违规计分的用例层（ADR-0035）。
//
// 一条链：记一条事件 → 重算有效分 → 按档位给处罚 → 通知用户。
// 每一步都可单独观察，所以"为什么我被禁言"能用一句日志回答清楚。
package app

import (
	"context"
	"fmt"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/moderation/domain"
)

// Service 是违规计分服务。
type Service struct {
	repo     domain.Repository
	account  domain.Account
	notifier domain.Notifier
	clock    domain.Clock
	logger   domain.Logger
	// appeals 是申诉仓储（第二阶段能力，可为 nil）。
	appeals domain.AppealRepository
}

// New 构造服务。
func New(repo domain.Repository, account domain.Account, notifier domain.Notifier, clock domain.Clock, logger domain.Logger) *Service {
	return &Service{repo: repo, account: account, notifier: notifier, clock: clock, logger: logger}
}

// RecordInput 是记一条违规事件的输入。
type RecordInput struct {
	UserID ctype.ID
	Level  domain.Level
	Source domain.Source
	// RefType / RefID 标识"是哪一件事"（内容单元），用于幂等。
	RefType string
	RefID   ctype.ID
	// Evidence 是一句人能读懂的依据（不回显命中词，避免词表泄漏）。
	Evidence string
	IP       string
	Device   string
}

// RecordResult 是一次记账的结果。
type RecordResult struct {
	// Inserted 为 false 表示这条事件之前已经记过（幂等命中，什么都没变）。
	Inserted bool
	// Score 是当前有效分。
	Score float64
	// Punishment 是这一轮落地的处罚档位。
	Punishment domain.Punishment
	// MuteDays 是实际禁言天数（0 表示没禁言）。
	MuteDays int
	// Skipped 非空表示这次没有计分，值是原因（例如管理员豁免）。
	Skipped string
}

// Record 记一条违规事件，并按需要施加处罚与通知。
func (s *Service) Record(ctx context.Context, in RecordInput) (*RecordResult, error) {
	if in.UserID.IsZero() {
		return nil, apperr.New(apperr.CodeUserNotFound)
	}

	// 管理员豁免：站长自己被扣分、被封禁没有任何意义，还会让后台直接不可用。
	if isAdmin, err := s.account.IsAdmin(ctx, in.UserID); err != nil {
		return nil, err
	} else if isAdmin {
		return &RecordResult{Skipped: "管理员账号不记分"}, nil
	}

	now := s.clock.Now()
	created, inserted, err := s.repo.Append(ctx, domain.Event{
		UserID:    in.UserID,
		Level:     in.Level,
		Score:     in.Level.Score(),
		Source:    in.Source,
		RefType:   in.RefType,
		RefID:     in.RefID,
		Evidence:  in.Evidence,
		Status:    domain.StatusActive,
		IP:        in.IP,
		Device:    in.Device,
		CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	if !inserted {
		// 幂等命中：同一件事重复判定，分数不变、不重复通知。
		return &RecordResult{Inserted: false}, nil
	}

	return s.evaluate(ctx, in.UserID, created, in.Level)
}

// evaluate 重算分数、施加处罚、发通知。
func (s *Service) evaluate(ctx context.Context, userID ctype.ID, trigger domain.Event, level domain.Level) (*RecordResult, error) {
	current := s.clock.Now()

	events, err := s.repo.ListByUser(ctx, userID, 200)
	if err != nil {
		return nil, err
	}
	score := domain.EffectiveScore(events, current)
	// 这条事件发生**之前**的分数：用来判断"这次违规有没有自己把档位推上去"。
	//
	// 为什么要它：封禁期内再违规升一档，但不能升两次 —— 如果新增的分本身就
	// 把档位从 1 天推到 7 天，再按"禁言期"额外升一档就变成 30 天，
	// 一次违规跳两级，用户完全无法预期。只有当分数没跨档时才补升一级。
	prior := domain.EffectiveScore(withoutEvent(events, trigger.ID), current)

	muted, err := s.account.IsMuted(ctx, userID)
	if err != nil {
		// 判定禁言状态失败不该让"记账"这件事失败：分数已经落库，
		// 处罚下一轮还能补。这里只记日志并按"未禁言"处理。
		s.warnf("读取禁言状态失败（按未禁言处理）: user=%d err=%v", userID, err)
		muted = false
	}

	punishment := decideEscalated(score, prior, muted)
	result := &RecordResult{Inserted: true, Score: score, Punishment: punishment}

	if days := punishment.MuteDays(); days > 0 {
		if err := s.account.Mute(ctx, userID, days); err != nil {
			s.warnf("施加禁言失败: user=%d days=%d err=%v", userID, days, err)
		} else {
			result.MuteDays = days
		}
	}

	s.notify(ctx, userID, level, score, punishment)
	return result, nil
}

// decideEscalated 决定最终档位。
//
// 规则：分数自己跨了档 → 用分数给的档；分数没跨档但人在禁言期 → 补升一档。
func decideEscalated(score, prior float64, muted bool) domain.Punishment {
	current := domain.Decide(score)
	if !muted {
		return current
	}
	if current != domain.Decide(prior) {
		// 这次违规已经把档位推上去了，不再额外升。
		return current
	}
	if current == domain.PunishmentNone {
		// 分数还没到线，但人在禁言期又犯：至少推一档，这是"再犯"的信号。
		return domain.PunishmentNone.Next()
	}
	return current.Next()
}

// withoutEvent 去掉指定事件后的账本（重算"这次之前"的分数）。
func withoutEvent(events []domain.Event, id ctype.ID) []domain.Event {
	if id.IsZero() {
		return events
	}
	out := make([]domain.Event, 0, len(events))
	for _, event := range events {
		if event.ID == id {
			continue
		}
		out = append(out, event)
	}
	return out
}

// notify 发一条站内通知。
//
// 文案只给"分数 + 档位"，不回显命中词：词表一旦出现在通知里，
// 就等于把"该绕开哪些词"告诉了所有人（与 ADR-0032 同一口径）。
func (s *Service) notify(ctx context.Context, userID ctype.ID, level domain.Level, score float64, punishment domain.Punishment) {
	if s.notifier == nil {
		return
	}
	reason := describeLevel(level)
	content := fmt.Sprintf("你的内容触发%s，风险分 +%d，当前有效分 %.1f。", reason, level.Score(), score)
	if punishment == domain.PunishmentNone {
		content += "本次仅记录，不处罚。"
	} else {
		content += fmt.Sprintf("当前处罚：%s。如认为判定有误，可在 7 天内申诉。", punishment.LadderLabel())
	}
	if err := s.notifier.NotifySystem(ctx, userID, content, "查看我的违规记录", "/studio/settings"); err != nil {
		s.warnf("发送违规通知失败: user=%d err=%v", userID, err)
	}
}

// describeLevel 返回给用户看的原因短语。
func describeLevel(level domain.Level) string {
	switch level {
	case domain.LevelKeyword:
		return "内容过滤规则"
	case domain.LevelImage:
		return "图片内容审核"
	case domain.LevelGeneral:
		return "人工内容复核（一般违规）"
	case domain.LevelSevere:
		return "人工内容复核（严重违规）"
	default:
		return "内容规则"
	}
}

// Summary 是一个用户的计分概览。
type Summary struct {
	UserID     ctype.ID
	Score      float64
	Punishment domain.Punishment
	Events     []domain.Event
}

// Summary 返回某用户的有效分与事件时间线。
func (s *Service) Summary(ctx context.Context, userID ctype.ID) (*Summary, error) {
	if userID.IsZero() {
		return nil, apperr.New(apperr.CodeUserNotFound)
	}
	events, err := s.repo.ListByUser(ctx, userID, 100)
	if err != nil {
		return nil, err
	}
	score := domain.EffectiveScore(events, s.clock.Now())
	return &Summary{
		UserID:     userID,
		Score:      score,
		Punishment: domain.Decide(score),
		Events:     events,
	}, nil
}

// Revoke 撤销一条事件（申诉通过 / 管理员纠错），返回撤销后的有效分。
func (s *Service) Revoke(ctx context.Context, eventID, operator ctype.ID, note string) (float64, error) {
	event, ok, err := s.repo.FindByID(ctx, eventID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, apperr.New(apperr.CodeModerationEventNotFound)
	}
	if event.Status == domain.StatusRevoked {
		// 重复撤销按幂等处理：结果一致，不该报错。
		return s.scoreOf(ctx, event.UserID), nil
	}
	if err := s.repo.Revoke(ctx, eventID, operator, note, s.clock.Now()); err != nil {
		return 0, err
	}
	return s.scoreOf(ctx, event.UserID), nil
}

// scoreOf 重算某用户的有效分。
func (s *Service) scoreOf(ctx context.Context, userID ctype.ID) float64 {
	events, err := s.repo.ListByUser(ctx, userID, 200)
	if err != nil {
		return 0
	}
	return domain.EffectiveScore(events, s.clock.Now())
}

func (s *Service) warnf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Warnf(format, args...)
	}
}
