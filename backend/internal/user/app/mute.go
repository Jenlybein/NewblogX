package app

import (
	"context"
	"time"

	"myblogx/internal/user/domain"
)

// 本文件是禁言的用例（票 399 / ADR-0032 决策 8）。
//
// 禁言 = 账号上的一个**时间点**（user_models.mute_until），不是第三个 Status：
//   - 判定：发布类动作读 now < mute_until（惰性，见 domain.CanPublish）；
//   - 恢复：到期后自动恢复，不需要人工解除；每小时对账只负责把列清空；
//   - 提前解除：管理员显式清空列（Unmute），同样不需要状态机。
//
// 写入口只有这三个：Mute（风险页处置用）、Unmute（账号管理页用）、
// ClearExpiredMutes（每小时对账用）。

// Mute 给目标账号设一次禁言，返回更新后的用户。
//
// days 的校验在 domain.NormalizeMuteDays：0 表示"没传"按默认 7 天，越界报
// user.mute_days_invalid。权限复用 CanChangeStatus：只有管理员能操作，
// 且不能禁言自己（管理员把自己禁了就没有人能解，与封禁同理）。
func (s *Service) Mute(ctx context.Context, viewer domain.Viewer, target domain.UserID, days int) (domain.User, error) {
	muteDays, err := domain.NormalizeMuteDays(days)
	if err != nil {
		return domain.User{}, err
	}
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.CanChangeStatus(u, viewer); err != nil {
		return domain.User{}, err
	}
	until := domain.MuteUntilAt(s.clock.Now(), muteDays)
	if err := s.users.SetMuteUntil(ctx, target, &until); err != nil {
		return domain.User{}, err
	}
	u.MuteUntil = &until
	return u, nil
}

// Unmute 提前解除禁言（清空 mute_until），返回更新后的用户。
//
// 幂等：本来就没禁言直接返回当前用户，不写库。权限与 Mute 同一口径。
func (s *Service) Unmute(ctx context.Context, viewer domain.Viewer, target domain.UserID) (domain.User, error) {
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.CanChangeStatus(u, viewer); err != nil {
		return domain.User{}, err
	}
	if u.MuteUntil == nil || u.MuteUntil.IsZero() {
		return u, nil
	}
	if err := s.users.SetMuteUntil(ctx, target, nil); err != nil {
		return domain.User{}, err
	}
	u.MuteUntil = nil
	return u, nil
}

// ClearExpiredMutes 是每小时对账的任务体：把已到期的禁言清空，返回清掉的条数。
//
// 与惰性判定结果一致：mute_until <= now 的行，CanPublish 早就判为可发布，
// 对账只是把它从表里擦掉（幂等、可重复跑）。now 由调用方传入，测试可钉住边界。
func (s *Service) ClearExpiredMutes(ctx context.Context, now time.Time) (int64, error) {
	return s.users.ClearExpiredMutes(ctx, now)
}
