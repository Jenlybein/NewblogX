package app

import (
	"context"
	"time"

	"myblogx/internal/user/domain"
)

// 本文件是「永久禁言」的用例（管理端评论页「禁言」下拉框里的永久档）。
//
// 为什么单独一个方法而不是复用 Mute(days)：
// Mute 的天数归一化上限是 365 天（ADR-0032 决策 8 的阶梯处罚口径，见
// domain.NormalizeMuteDays）。把"永久"塞成 3650 天会被归一化直接拒绝；
// 压成 365 天又会让返回给管理员的 mute_until 变成一句谎话（"一年后自动恢复"）。
// 于是永久档走这条独立通道：到期时间落到 MySQL DATETIME 的表示上界。
//
// 解除仍然是 Unmute（清空 mute_until）：禁言依然只是账号上的一个时间点，
// 没有第三个 Status，"永久"也不是新状态。

// permanentMuteUntil 是「永久禁言」写入的到期时间。
//
// 取 DATETIME 的表示上界（9999-12-31）而不是 time.Time 的零值或 MaxInt64
// 纳秒：零值会被 CanPublish 判成"早就到期"，纳秒上界无法安全落库。这里刻意
// 用当天中午而不是 23:59:59 —— 驱动按 DSN 的 loc 换算时间（dsn 里是 loc=Local），
// 上界时刻留出 ±12 小时的时区余量，换算后不会溢出到 year 10000。
//
// 语义上它是"没有到期日"：惰性判定 now < until 永远放不过去；每小时的到期
// 对账也不会把它当成已到期清掉（那是 <= now 才清）。
var permanentMuteUntil = time.Date(9999, 12, 31, 12, 0, 0, 0, time.UTC)

// MuteForever 给目标账号设一次没有到期日的禁言，返回更新后的用户。
//
// 权限与 Mute 同一口径（domain.CanChangeStatus：只有管理员能操作，且不能操作
// 自己）：永久禁言比临时禁言更重，不该有一条更松的入口。解除照旧走 Unmute。
func (s *Service) MuteForever(ctx context.Context, viewer domain.Viewer, target domain.UserID) (domain.User, error) {
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.CanChangeStatus(u, viewer); err != nil {
		return domain.User{}, err
	}
	until := permanentMuteUntil
	if err := s.users.SetMuteUntil(ctx, target, &until); err != nil {
		return domain.User{}, err
	}
	u.MuteUntil = &until
	return u, nil
}
