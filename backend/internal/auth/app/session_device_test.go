package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/auth/app"
	"myblogx/internal/auth/domain"
)

// 本文件钉住「同设备再次登录自动吊销旧会话」（#131）。
//
// 为什么要有这一条：登录设备列表是按设备聚合展示的，但如果旧会话仍然有效，
// 聚合只是「看起来一条」——库里还留着多条可换令牌的活会话。安全上这是
// "同设备重复登录"留下的等价凭证，应当被新的登录挤掉。

// TestReloginOnSameDeviceRevokesOldSession 同一设备再次登录后只剩新会话有效。
func TestReloginOnSameDeviceRevokesOldSession(t *testing.T) {
	t.Parallel()
	r := setup(t)

	first := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
	r.Clock().Advance(time.Hour)
	second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAChromeWindows})

	if first.Session.ID == second.Session.ID {
		t.Fatalf("两次登录应产生不同会话")
	}

	// 旧会话必须已经不可用（拿它去刷新应当失败）。
	if _, err := r.Service().Refresh(
		context.Background(), first.Tokens.Refresh, domain.Meta{}); err == nil {
		t.Fatalf("同设备旧会话应已被吊销，但其刷新令牌仍然可用")
	}

	// 新会话必须可用。
	if _, err := r.Service().Refresh(
		context.Background(), second.Tokens.Refresh, domain.Meta{}); err != nil {
		t.Fatalf("同设备新会话不应被自己吊销: %v", err)
	}

	// 聚合后只剩一条，且累计次数为 1——证明旧会话是真的被吊销了，
	// 而不是仅仅被聚合掩盖。
	list := mustList(t, r, second.Session.ID)
	if len(list) != 1 {
		t.Fatalf("同设备应只剩 1 条设备记录，实际 %d 条: %+v", len(list), list)
	}
	if list[0].LoginCount != 1 {
		t.Fatalf("旧会话已被吊销，累计次数应为 1，实际 %d", list[0].LoginCount)
	}
	if list[0].SessionID != second.Session.ID {
		t.Fatalf("当前设备应指向新会话，got=%v want=%v", list[0].SessionID, second.Session.ID)
	}
}

// TestReloginOnDifferentDeviceKeepsBoth 不同设备登录不受影响。
func TestReloginOnDifferentDeviceKeepsBoth(t *testing.T) {
	t.Parallel()
	r := setup(t)

	first := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
	r.Clock().Advance(time.Hour)
	second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAFirefoxLinux})

	// 两台设备各自一条，两条会话都还能换令牌。
	if _, err := r.Service().Refresh(
		context.Background(), first.Tokens.Refresh, domain.Meta{}); err != nil {
		t.Fatalf("另一台设备不应被吊销: %v", err)
	}
	list := mustList(t, r, second.Session.ID)
	if len(list) != 2 {
		t.Fatalf("两台设备应有 2 条，实际 %d 条: %+v", len(list), list)
	}
}

// TestReloginWithBlankUADoesNotRevokeOthers 空 UA 无法识别设备，不做自动吊销。
//
// 空 UA 的设备键会退化成同一个常量，若照样按设备组吊销，会把**别的**客户端
// 会话一起挤掉。宁可不自动吊销，也不能误杀。
func TestReloginWithBlankUADoesNotRevokeOthers(t *testing.T) {
	t.Parallel()
	r := setup(t)

	first := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: ""})
	r.Clock().Advance(time.Hour)
	loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: ""})

	if _, err := r.Service().Refresh(
		context.Background(), first.Tokens.Refresh, domain.Meta{}); err != nil {
		t.Fatalf("空 UA 时不应自动吊销旧会话: %v", err)
	}
}

// TestRevokeSessionByDeviceStillWorks 按会话下线单个设备的能力仍可用。
func TestRevokeSessionByDeviceStillWorks(t *testing.T) {
	t.Parallel()
	r := setup(t)

	chrome := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
	r.Clock().Advance(time.Hour)
	firefox := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAFirefoxLinux})

	if err := r.Service().RevokeSession(
		context.Background(), chrome.Session.UserID.Uint64(), chrome.Session.ID.Uint64()); err != nil {
		t.Fatalf("下线设备失败: %v", err)
	}

	// 被下线的设备不可再刷新，另一台不受影响。
	if _, err := r.Service().Refresh(
		context.Background(), chrome.Tokens.Refresh, domain.Meta{}); err == nil {
		t.Fatalf("被下线的设备其刷新令牌应失效")
	}
	if _, err := r.Service().Refresh(
		context.Background(), firefox.Tokens.Refresh, domain.Meta{}); err != nil {
		t.Fatalf("另一台设备不应被牵连: %v", err)
	}
	if list := mustList(t, r, firefox.Session.ID); len(list) != 1 {
		t.Fatalf("下线一台后应剩 1 条，实际 %d 条: %+v", len(list), list)
	}
}

// loginMeta 是给对方用例复用的空 meta 占位（避免各处写字面量）。
var _ = app.LoginInput{}
