package app

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/apptest"
	userdomain "myblogx/internal/user/domain"
)

// 风险页账号处置桥（票 399 / ADR-0032 决策 8）。
//
// 两个动作必须走 user 模块既有用例（权限、会话吊销、时长校验都在那里）：
//   - ban = 封禁（账号状态 3），登录与发布都被拦；
//   - mute = 禁言，只写 mute_until（不动会话），到期惰性恢复。

const accountActor = ctype.ID("9001")

// seedPunishableUser 造一个可被处置的普通账号。
func seedPunishableUser(t *testing.T, repos *apptest.Repos) userdomain.User {
	t.Helper()
	return repos.Users().Seed(userdomain.User{
		ID:       ctype.NewID(2001),
		Username: "spammer",
		Nickname: "刷子",
		Status:   userdomain.StatusActive,
		Role:     userdomain.RoleUser,
	})
}

func TestAccountDisposerBridgeBanDisablesAccount(t *testing.T) {
	t.Parallel()

	svc, repos := newGateUserService()
	target := seedPunishableUser(t, repos)

	actor := ctype.ID(accountActor)
	if err := (accountDisposerBridge{users: svc}).BanUser(context.Background(), actor, target.ID); err != nil {
		t.Fatalf("封禁失败: %v", err)
	}
	got, err := repos.Users().FindByID(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Status != userdomain.StatusBanned {
		t.Fatalf("封禁后账号状态应为封禁，实际 %v", got.Status)
	}
	// 封禁 = 登录与发布都被拦。
	if err := userdomain.CanLogin(got); !apperr.IsCode(err, apperr.CodeUserAccountBanned) {
		t.Fatalf("封禁后登录应被拒（account_banned），实际 %v", err)
	}
}

func TestAccountDisposerBridgeMuteSetsUntil(t *testing.T) {
	t.Parallel()

	svc, repos := newGateUserService()
	target := seedPunishableUser(t, repos)

	if err := (accountDisposerBridge{users: svc}).MuteUser(context.Background(), ctype.ID(accountActor), target.ID, 7); err != nil {
		t.Fatalf("禁言失败: %v", err)
	}
	got, err := repos.Users().FindByID(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.MuteUntil == nil {
		t.Fatal("禁言后必须写 mute_until")
	}
	// 7 天 = baseTime + 168h（user 模块的时钟）。
	if want := gateBaseTime.Add(7 * 24 * time.Hour); !got.MuteUntil.Equal(want) {
		t.Fatalf("禁言到期时间应为 %v，实际 %v", want, got.MuteUntil)
	}
	if got.Status != userdomain.StatusActive {
		t.Fatalf("禁言不该改账号状态，实际 %v", got.Status)
	}
	// 缺省天数（0）按 7 天处理，越界被拒。
	if err := (accountDisposerBridge{users: svc}).MuteUser(context.Background(), ctype.ID(accountActor), target.ID, 0); err != nil {
		t.Fatalf("缺省天数禁言应成功: %v", err)
	}
	err = (accountDisposerBridge{users: svc}).MuteUser(context.Background(), ctype.ID(accountActor), target.ID, 366)
	if !apperr.IsCode(err, apperr.CodeUserMuteDaysInvalid) {
		t.Fatalf("天数越界应被拒（mute_days_invalid），实际 %v", err)
	}
}
