package app

import (
	"context"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/moderation/domain"
)

// 本文件是申诉的用例（ADR-0035）：用户提交、管理员处置。
//
// 设计上只有一条硬约束：**通过申诉必须真的把后果退回去** —— 事件置撤销、
// 分数重算、禁言解除。只把申诉单标成"已通过"而分数还挂着，是最糟的实现：
// 用户看到"申诉成功"但依然发不了东西。

// SetAppeals 注入申诉仓储。
//
// 单独一个 setter 而不是塞进 New 的参数表：申诉是第二阶段能力，媒体/过滤
// 的上报口只需要 Record/Summary，不该被申诉的依赖绑架。
func (s *Service) SetAppeals(repo domain.AppealRepository) { s.appeals = repo }

// SubmitAppeal 提交一条申诉。
func (s *Service) SubmitAppeal(ctx context.Context, userID, eventID ctype.ID, reason string) (*domain.Appeal, error) {
	if s.appeals == nil {
		return nil, apperr.New(apperr.CodeModerationAppealInvalid)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, apperr.NewWithMessage(apperr.CodeModerationAppealInvalid, "申诉必须填写理由")
	}

	event, ok, err := s.repo.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.New(apperr.CodeModerationEventNotFound)
	}
	if event.UserID != userID {
		// 只能给自己的处罚申诉。用"事件不存在"而不是"无权"，避免探测别人有没有违规。
		return nil, apperr.New(apperr.CodeModerationEventNotFound)
	}
	if !domain.CanAppeal(event, s.clock.Now()) {
		return nil, apperr.NewWithMessage(apperr.CodeModerationAppealInvalid,
			"只能对 7 天内的、仍然生效的违规事件申诉")
	}

	appeal, err := s.appeals.Create(ctx, domain.Appeal{
		EventID:   eventID,
		UserID:    userID,
		Reason:    reason,
		CreatedAt: s.clock.Now(),
	})
	if err != nil {
		// 唯一键冲突：同一事件重复申诉。
		return nil, apperr.NewWithMessage(apperr.CodeModerationAppealInvalid, "这条处罚已经申诉过了")
	}
	return &appeal, nil
}

// ListAppeals 按状态列出申诉（管理员）。
func (s *Service) ListAppeals(ctx context.Context, status domain.AppealStatus) ([]domain.Appeal, error) {
	if s.appeals == nil {
		return nil, nil
	}
	return s.appeals.List(ctx, status, 50)
}

// HandleAppeal 处置一条申诉。
//
// approve=true：撤销事件、重算分数、解除禁言、恢复内容（内容是否恢复由各自模块
// 决定，这里只负责把账号侧后果退回去）。
// approve=false：驳回，note 必填 —— 用户看到的就是这句话。
func (s *Service) HandleAppeal(ctx context.Context, appealID, operator ctype.ID, approve bool, note string) (*domain.Appeal, error) {
	if s.appeals == nil {
		return nil, apperr.New(apperr.CodeModerationAppealInvalid)
	}
	note = strings.TrimSpace(note)
	if !approve && note == "" {
		return nil, apperr.NewWithMessage(apperr.CodeModerationAppealInvalid, "驳回必须填写理由")
	}

	appeal, ok, err := s.appeals.FindByID(ctx, appealID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.New(apperr.CodeModerationEventNotFound)
	}
	if appeal.Status != domain.AppealPending {
		return nil, apperr.NewWithMessage(apperr.CodeModerationAppealInvalid, "这条申诉已经处理过了")
	}

	status := domain.AppealRejected
	if approve {
		status = domain.AppealApproved
	}
	now := s.clock.Now()
	if err := s.appeals.Handle(ctx, appealID, operator, status, note, now); err != nil {
		return nil, err
	}
	appeal.Status = status
	appeal.Note = note
	appeal.HandledBy = operator
	appeal.HandledAt = &now

	if !approve {
		s.notifyAppealResult(ctx, appeal.UserID, false, note)
		return &appeal, nil
	}

	// 通过：撤销事件 + 解除禁言。
	if err := s.repo.Revoke(ctx, appeal.EventID, operator, "申诉通过", now); err != nil {
		return nil, err
	}
	score := s.scoreOf(ctx, appeal.UserID)
	// 撤销后分数可能已经降到处罚线以下，这时应该解禁；否则保留（由下一轮判定处理）。
	if domain.Decide(score) == domain.PunishmentNone {
		if err := s.account.Unmute(ctx, appeal.UserID); err != nil {
			s.warnf("申诉通过后解除禁言失败: user=%d err=%v", appeal.UserID, err)
		}
	}
	s.notifyAppealResult(ctx, appeal.UserID, true, note)
	return &appeal, nil
}

// notifyAppealResult 把处置结果告诉用户。
func (s *Service) notifyAppealResult(ctx context.Context, userID ctype.ID, approved bool, note string) {
	if s.notifier == nil {
		return
	}
	content := "你的申诉已通过：相关违规记录已撤销，风险分已重算。"
	if !approved {
		content = "你的申诉未通过。"
		if note != "" {
			content += "理由：" + note
		}
	}
	if err := s.notifier.NotifySystem(ctx, userID, content, "查看我的违规记录", "/studio/settings"); err != nil {
		s.warnf("发送申诉结果通知失败: user=%d err=%v", userID, err)
	}
}
