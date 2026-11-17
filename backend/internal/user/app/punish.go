package app

import (
	"context"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/domain"
)

// 本文件是"违规计分"对账号模块的窄接口（ADR-0035）。
//
// 计分模块需要三件事：这个账号是不是管理员（豁免）、现在是不是在禁言期
// （"封禁期内再违规升一档"）、以及施加一次禁言。
//
// 为什么单独一个方法而不是复用 Mute(viewer, ...)：Mute 要求传一个管理员
// 的 Viewer（CanChangeStatus 会拦"不能禁自己"），而这里的发起方是**系统**
// （审核判定），没有请求上下文、也不该被"不能禁自己"这条规则挡住在
// 管理员账号上 —— 那条规则已经在计分入口用"管理员不记分"提前处理掉了。

// IsAdminAccount 判断目标账号是不是管理员。
func (s *Service) IsAdminAccount(ctx context.Context, userID ctype.ID) (bool, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return u.Role.IsAdmin(), nil
}

// IsAccountMuted 判断目标账号当前是否在禁言期。
func (s *Service) IsAccountMuted(ctx context.Context, userID ctype.ID) (bool, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return u.IsMuted(s.clock.Now()), nil
}

// RelieveMute 由系统解除禁言（申诉通过后把账号侧后果退回去）。
//
// 与 Unmute 的区别：不做 Viewer 权限检查（发起方是申诉处置流程，不是某个用户在点按钮）。
func (s *Service) RelieveMute(ctx context.Context, userID ctype.ID) error {
	return s.users.SetMuteUntil(ctx, userID, nil)
}

// PunishMute 由系统施加一次禁言（违规计分的阶梯处罚调用）。
//
// 与 Mute 共用同一条落库路径（SetMuteUntil）与同一份天数归一化，区别只是
// 不做 Viewer 权限检查：调用方是审核判定，不是某个用户在点按钮。
func (s *Service) PunishMute(ctx context.Context, userID ctype.ID, days int) error {
	muteDays, err := domain.NormalizeMuteDays(days)
	if err != nil {
		return err
	}
	until := domain.MuteUntilAt(s.clock.Now(), muteDays)
	return s.users.SetMuteUntil(ctx, userID, &until)
}
