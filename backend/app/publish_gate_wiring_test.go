package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	userapp "myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	userdomain "myblogx/internal/user/domain"
)

// 发布闸门的装配桥（票 399 / ADR-0032 决策 8）。
//
// 各内容模块只依赖"问一句能不能发布"的窄端口，判定与文案全在 user 模块。
// 这里钉住桥本身的三个行为：禁言被拒、到期放行、装配缺失时失败关闭。

var gateBaseTime = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// newGateUserService 构造一个真实的 user 用例服务（内存仓储），并返回仓储。
func newGateUserService() (*userapp.Service, *apptest.Repos) {
	repos := apptest.NewRepos(gateBaseTime)
	return repos.Service(), repos
}

func TestPublishGateBridgeRejectsMutedUser(t *testing.T) {
	t.Parallel()

	svc, repos := newGateUserService()
	until := gateBaseTime.Add(7 * 24 * time.Hour)
	repos.Users().Seed(userdomain.User{
		ID: ctype.NewID(1001), Username: "alice", Nickname: "爱丽丝",
		Status: userdomain.StatusActive, MuteUntil: &until,
	})

	gate := publishGateBridge{users: svc}
	err := gate.EnsureCanPublish(context.Background(), ctype.NewID(1001))
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("被禁言者应被拒（user.muted），实际 %v", err)
	}
	// 桥不翻译错误：文案直接来自 user 模块，必须带到期时间。
	if err == nil || !strings.Contains(err.Error(), until.Format("2006-01-02 15:04")) {
		t.Fatalf("错误文案应带到期时间，实际 %v", err)
	}
}

func TestPublishGateBridgeAllowsExpiredMute(t *testing.T) {
	t.Parallel()

	svc, repos := newGateUserService()
	expired := gateBaseTime.Add(-time.Hour)
	repos.Users().Seed(userdomain.User{
		ID: ctype.NewID(1001), Username: "alice", Nickname: "爱丽丝",
		Status: userdomain.StatusActive, MuteUntil: &expired,
	})

	gate := publishGateBridge{users: svc}
	if err := gate.EnsureCanPublish(context.Background(), ctype.NewID(1001)); err != nil {
		t.Fatalf("禁言到期后应放行（惰性判定），实际 %v", err)
	}
}

func TestPublishGateBridgeFailsClosedWithoutUsers(t *testing.T) {
	t.Parallel()

	gate := publishGateBridge{}
	if err := gate.EnsureCanPublish(context.Background(), ctype.NewID(1001)); err == nil {
		t.Fatal("装配缺失时必须失败关闭（静默放行等于禁言失效）")
	}
}

// TestWirePublishGateIsSafeWithoutServices 缺服务时不 panic、不误改（进程角色不同
// 装配并不相同：cron / worker 没有这些服务）。
func TestWirePublishGateIsSafeWithoutServices(t *testing.T) {
	t.Parallel()

	WirePublishGate(nil, nil)
	WirePublishGate(&Infra{}, nil)
}
