package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/app"
	"myblogx/internal/auth/apptest"
	"myblogx/internal/auth/domain"
	"myblogx/internal/kernel/ctype"
)

// 与 domain 层用例同一批真实 UA 样本，用于验证"按设备聚合"的对外呈现。
const (
	testUAChromeWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	testUAFirefoxLinux  = "Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0"
)

func loginAs(t *testing.T, r *apptest.Repos, meta domain.Meta) *app.LoginResult {
	t.Helper()
	res, err := r.Service().Login(context.Background(), app.LoginInput{
		Account: "alice", Password: alicePw, Meta: meta,
	})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	return res
}

// TestListDevices 覆盖登录设备列表的聚合口径：同设备一条、异设备各一条、
// 首次登录 / 最近活跃 / 累计次数、当前设备标记。
func TestListDevices(t *testing.T) {
	t.Run("同一 UA 多次登录聚合为一条", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
		r.Clock().Advance(time.Hour)
		second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAChromeWindows})

		list := mustList(t, r, second.Session.ID)
		if len(list) != 1 {
			t.Fatalf("同一设备应聚合为 1 条，实际 %d 条: %+v", len(list), list)
		}
		got := list[0]
		// #131 之后同设备再次登录会**吊销**旧会话，所以累计次数是 1 而不是 2：
		// 旧会话已不在"有效会话"集合里，聚合也就只剩一条。这正是要的语义——
		// 同设备重复登录不该在库里留下多条可换令牌的活会话。
		if got.LoginCount != 1 {
			t.Fatalf("旧会话已被吊销，累计次数 got=%d want=1", got.LoginCount)
		}
		// 首次登录时间仍来自最早的会话记录（被吊销的会话不参与聚合后，
		// 只剩新会话，因此首次=最近=本次登录时间）。
		if !got.FirstSeenAt.Equal(baseTime.Add(time.Hour)) {
			t.Fatalf("首次登录时间 got=%v want=%v", got.FirstSeenAt, baseTime.Add(time.Hour))
		}
		if !got.LastSeenAt.Equal(baseTime.Add(time.Hour)) {
			t.Fatalf("最近活跃时间 got=%v want=%v", got.LastSeenAt, baseTime.Add(time.Hour))
		}
		// 展示用的 IP / 地域 / UA 取最近一次登录，而不是首次。
		if got.IP != "2.2.2.2" || got.Addr != "上海" || got.UA != testUAChromeWindows {
			t.Fatalf("应保留最近一次登录的客户端信息，实际 ip=%q addr=%q ua=%q", got.IP, got.Addr, got.UA)
		}
		if got.Device.Browser != "Chrome" || got.Device.OS != "Windows" {
			t.Fatalf("设备描述解析错误: %+v", got.Device)
		}
		if strings.Contains(got.Device.Label(), "Mozilla/") {
			t.Fatalf("设备描述不应是 UA 原文: %q", got.Device.Label())
		}
		if got.SessionID != second.Session.ID {
			t.Fatalf("当前设备应作为代表会话，got=%v want=%v", got.SessionID, second.Session.ID)
		}
	})

	t.Run("不同设备各自一条", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
		r.Clock().Advance(time.Hour)
		second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAFirefoxLinux})

		list := mustList(t, r, second.Session.ID)
		if len(list) != 2 {
			t.Fatalf("两台不同设备应有 2 条，实际 %d 条: %+v", len(list), list)
		}
		// 最近活跃的排在前面。
		if !list[0].LastSeenAt.Equal(baseTime.Add(time.Hour)) {
			t.Fatalf("最近活跃的设备应排第一，实际首条=%+v", list[0])
		}
		seen := map[string]bool{}
		for _, d := range list {
			seen[d.Device.Browser+"|"+d.Device.OS] = true
			if d.LoginCount != 1 {
				t.Fatalf("不同设备各 1 次登录，实际 %+v", d)
			}
		}
		if !seen["Chrome|Windows"] || !seen["Firefox|Linux"] {
			t.Fatalf("设备集合不完整: %v", seen)
		}
	})

	t.Run("当前设备标记基于会话 ID", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
		r.Clock().Advance(time.Hour)
		second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAChromeWindows})

		// #131 之后同设备再次登录会吊销旧会话，所以"当前会话是较早那条"的
		// 情形在真实登录路径上已不可达。这里改为断言新契约：无论拿哪条会话
		// 当"当前"，代表会话都只能是仍在有效集合里的新会话。
		list := mustList(t, r, first.Session.ID)
		if len(list) != 1 || list[0].SessionID != second.Session.ID {
			t.Fatalf("旧会话已被吊销，代表会话应为新会话: %+v (second=%v)", list, second.Session.ID)
		}

		list = mustList(t, r, second.Session.ID)
		if len(list) != 1 || list[0].SessionID != second.Session.ID {
			t.Fatalf("当前设备代表会话错误: %+v", list)
		}
	})

	t.Run("空 UA 不崩且可聚合", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: ""})
		r.Clock().Advance(time.Hour)
		second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: ""})

		list := mustList(t, r, second.Session.ID)
		if len(list) != 1 {
			t.Fatalf("同一空 UA 应聚合为 1 条，实际 %d", len(list))
		}
		if list[0].Device.Label() != "未知设备" {
			t.Fatalf("空 UA 的设备描述应为'未知设备'，实际 %q", list[0].Device.Label())
		}
	})

	t.Run("异常 UA 不崩且不同 UA 各自一条", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		loginAs(t, r, domain.Meta{UA: "UA-A"})
		r.Clock().Advance(time.Hour)
		loginAs(t, r, domain.Meta{UA: "UA-B"})
		r.Clock().Advance(time.Hour)
		third := loginAs(t, r, domain.Meta{UA: strings.Repeat("Z", 4096)})

		list := mustList(t, r, third.Session.ID)
		if len(list) != 3 {
			t.Fatalf("3 种不同 UA 应有 3 条，实际 %d: %+v", len(list), list)
		}
		for _, d := range list {
			if d.Device.Label() != "未知设备" {
				t.Fatalf("异常 UA 的设备描述应为'未知设备'，实际 %q", d.Device.Label())
			}
		}
	})
}

// 设备聚合后「下线设备」必须下掉整台设备，而不是只吊销代表会话——
// 否则刷新后同一设备会带着剩余会话重新出现。
func TestRevokeDeviceRemovesWholeGroup(t *testing.T) {
	t.Parallel()
	r := setup(t)
	loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})
	r.Clock().Advance(time.Hour)
	second := loginAs(t, r, domain.Meta{IP: "2.2.2.2", Addr: "上海", UA: testUAChromeWindows})
	r.Clock().Advance(time.Hour)
	other := loginAs(t, r, domain.Meta{IP: "3.3.3.3", Addr: "深圳", UA: testUAFirefoxLinux})

	if err := r.Service().RevokeSession(context.Background(), aliceID.Uint64(), second.Session.ID.Uint64()); err != nil {
		t.Fatalf("下线设备失败: %v", err)
	}

	list := mustList(t, r, other.Session.ID)
	if len(list) != 1 || list[0].Device.Browser != "Firefox" {
		t.Fatalf("应只剩另一台设备，实际 %+v", list)
	}
}

// 私信票据换发按会话 ID 精确判定，不能被"按设备聚合"误伤。
//
// #131 之后真实登录路径会吊销同设备的旧会话，所以这里**直接塞一条同设备的
// 第二会话**来构造原始场景：SessionActive 仍必须按 ID 精确回答，不能因为
// "这台设备已经有一条会话了"就否掉另一条。
func TestSessionActive(t *testing.T) {
	t.Parallel()
	r := setup(t)
	first := loginAs(t, r, domain.Meta{IP: "1.1.1.1", Addr: "杭州", UA: testUAChromeWindows})

	now := r.Clock().Now()
	extra := domain.Session{
		UserID:           first.Session.UserID,
		RefreshTokenHash: "extra-same-device-hash",
		IP:               "9.9.9.9",
		Addr:             "南京",
		UA:               testUAChromeWindows,
		CreatedAt:        now,
		LastSeenAt:       &now,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
	}
	if err := r.Sessions().Create(context.Background(), &extra); err != nil {
		t.Fatalf("插入同设备第二会话失败: %v", err)
	}
	second := &app.LoginResult{Session: extra}

	for _, id := range []ctype.ID{first.Session.ID, second.Session.ID} {
		active, err := r.Service().SessionActive(context.Background(), aliceID.Uint64(), id.Uint64())
		if err != nil || !active {
			t.Fatalf("会话 %v 应有效，实际 active=%v err=%v", id, active, err)
		}
	}

	active, err := r.Service().SessionActive(context.Background(), aliceID.Uint64(), 9999)
	if err != nil || active {
		t.Fatalf("不存在的会话应无效，实际 active=%v err=%v", active, err)
	}

	if err := r.Service().RevokeSession(context.Background(), aliceID.Uint64(), second.Session.ID.Uint64()); err != nil {
		t.Fatalf("下线设备失败: %v", err)
	}
	for _, id := range []ctype.ID{first.Session.ID, second.Session.ID} {
		active, err := r.Service().SessionActive(context.Background(), aliceID.Uint64(), id.Uint64())
		if err != nil || active {
			t.Fatalf("下线设备后会话 %v 应失效，实际 active=%v err=%v", id, active, err)
		}
	}
}

// 空用户 ID 仍按原口径拒绝（聚合不能绕过鉴权校验）。
func TestListDevicesRejectsZeroUser(t *testing.T) {
	t.Parallel()
	r := setup(t)
	_, err := r.Service().ListSessions(context.Background(), ctype.NewID(0), ctype.NewID(0))
	assertCode(t, err, apperr.CodeAuthRequired)
}
