package app_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// 永久禁言的用例（管理端评论页「禁言」下拉框的永久档）。
//
// 为什么单独一条通路：Mute(days) 的天数归一化上限是 365 天，塞不进"永久"。
// 这里钉住三件事：
//   - 永久禁言写进一个"看起来永不生效"的远期时间（现在是过去、将来也还是未来）；
//   - 权限与 Mute 同一口径（只有管理员、不能操作自己）；
//   - 解除仍然走 Unmute，永久禁言不是第三个账号状态。

func TestMuteForever_SetsFarFutureUntil(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)

	got, err := r.Service().MuteForever(testCtx(), admin, aliceID)
	if err != nil {
		t.Fatalf("管理员永久禁言不该失败: %v", err)
	}
	if got.MuteUntil == nil {
		t.Fatal("永久禁言后 MuteUntil 不能为空（否则惰性判定会立刻放行）")
	}
	// 几年之内都远远不到期：用 10 年后的时刻当判据，不必钉死具体年份。
	if !got.MuteUntil.After(baseTime.AddDate(10, 0, 0)) {
		t.Fatalf("永久禁言的到期时间应是远期，实际 %v", got.MuteUntil)
	}

	// 发布闸门真的拦住它（解除前一直禁言）。
	if err := r.Service().EnsureCanPublish(testCtx(), aliceID); !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("永久禁言期间发表应被拒，实际 %v", err)
	}

	// 解除仍走 Unmute：清空列，不留"永久"这个第三态。
	if _, err := r.Service().Unmute(testCtx(), admin, aliceID); err != nil {
		t.Fatalf("解除永久禁言失败: %v", err)
	}
	after, err := r.Users().FindByID(testCtx(), aliceID)
	if err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}
	if after.MuteUntil != nil {
		t.Fatalf("解除后 MuteUntil 应为 nil，实际 %v", after.MuteUntil)
	}
}

func TestMuteForever_RejectsNonAdminAndSelf(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)

	// 非管理员不能操作（与 Mute 同一口径）。
	if _, err := r.Service().MuteForever(testCtx(), alice, aliceID); !apperr.IsCode(err, apperr.CodeUserForbidden) {
		t.Fatalf("普通用户不能永久禁言，实际 %v", err)
	}

	// 管理员不能永久禁言自己：封住自己的发布能力之后没人能解，与封禁同理。
	boss := r.Users().Seed(domain.User{
		ID: adminID, Username: "root", Nickname: "站长",
		Status: domain.StatusActive, Role: domain.RoleAdmin,
	})
	if _, err := r.Service().MuteForever(testCtx(), admin, boss.ID); !apperr.IsCode(err, apperr.CodeUserStatusSelfLocked) {
		t.Fatalf("管理员不能永久禁言自己，实际 %v", err)
	}
}
