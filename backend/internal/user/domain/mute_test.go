package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// 禁言的领域不变量（票 399 / ADR-0032 决策 8）：
//   - IsMuted 的边界：nil / 零值 / 未到期 / 到期 / 刚好到期；
//   - 到期后 CanPublish 立刻放行（惰性判定，与对账任务跑没跑过无关）；
//   - 禁用 / 封禁优先于禁言（被封的人不该看到"禁言到某天"）；
//   - 错误文案带到期时间与剩余天数；
//   - 时长 1–365，缺省 7 天。

// at 返回 now 加减 d 的时间指针。
func at(d time.Duration) *time.Time {
	v := now.Add(d)
	return &v
}

func TestIsMuted(t *testing.T) {
	cases := []struct {
		name string
		user domain.User
		want bool
	}{
		{"未禁言（nil）", domain.User{Status: domain.StatusActive}, false},
		{"未禁言（零值时间）", domain.User{MuteUntil: &time.Time{}}, false},
		{"禁言未到期", domain.User{MuteUntil: at(time.Hour)}, true},
		{"禁言已到期", domain.User{MuteUntil: at(-time.Minute)}, false},
		{"恰好到期的那一毫秒", domain.User{MuteUntil: &now}, false},
	}
	for _, c := range cases {
		got := c.user.IsMuted(now)
		if got != c.want {
			t.Fatalf("%s: IsMuted=%v，期望 %v", c.name, got, c.want)
		}
	}
}

func TestMuteRemainingDays(t *testing.T) {
	cases := []struct {
		name string
		user domain.User
		want int
	}{
		{"未禁言", domain.User{}, 0},
		{"已到期", domain.User{MuteUntil: at(-time.Hour)}, 0},
		{"剩 3 天整", domain.User{MuteUntil: at(72 * time.Hour)}, 3},
		{"剩 2 天零 1 小时（向上取整到 3 天）", domain.User{MuteUntil: at(49 * time.Hour)}, 3},
		{"剩 1 小时（向上取整到 1 天）", domain.User{MuteUntil: at(time.Hour)}, 1},
	}
	for _, c := range cases {
		if got := c.user.MuteRemainingDays(now); got != c.want {
			t.Fatalf("%s: MuteRemainingDays=%d，期望 %d", c.name, got, c.want)
		}
	}
}

func TestCanPublish(t *testing.T) {
	cases := []struct {
		name string
		user domain.User
		want apperr.Code
	}{
		{"正常账号", activeUser(aliceID, domain.RoleUser), ""},
		{"禁言未到期", domain.User{Status: domain.StatusActive, MuteUntil: at(24 * time.Hour)}, apperr.CodeUserMuted},
		{"禁言已到期（惰性放行，不需要对账）", domain.User{Status: domain.StatusActive, MuteUntil: at(-time.Hour)}, ""},
		// 账号状态优先：被封的人先看到"账号已被封禁"，而不是"你被禁言到某天"。
		{"被封禁（即使禁言未到期）", domain.User{Status: domain.StatusBanned, MuteUntil: at(24 * time.Hour)}, apperr.CodeUserAccountBanned},
		{"被禁用（即使禁言未到期）", domain.User{Status: domain.StatusDisabled, MuteUntil: at(24 * time.Hour)}, apperr.CodeUserAccountDisabled},
	}
	for _, c := range cases {
		assertCode(t, domain.CanPublish(c.user, now), c.want)
	}
}

func TestMuteErrorMessageCarriesUntil(t *testing.T) {
	until := now.Add(48 * time.Hour)
	err := domain.CanPublish(domain.User{Status: domain.StatusActive, MuteUntil: &until}, now)
	if err == nil {
		t.Fatal("禁言未到期时 CanPublish 必须报错")
	}

	var muteErr *domain.MuteError
	if !errors.As(err, &muteErr) {
		t.Fatalf("应能 errors.As 取回 MuteError，实际 %T", err)
	}
	// 到期时间按分钟级格式写进文案：用户要的正是"什么时候恢复"。
	if want := until.Format("2006-01-02 15:04"); !strings.Contains(err.Error(), want) {
		t.Fatalf("文案应含到期时间 %q，实际 %q", want, err.Error())
	}
	if !strings.Contains(err.Error(), "还有 2 天") {
		t.Fatalf("文案应含剩余天数，实际 %q", err.Error())
	}
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("错误码应是 user.muted，实际 %v", err)
	}
	// 错误值转成 HTTP 文案时用的是 err.Error()（utils/validate 的 translateCommonError）。
	// 这里再钉一次静态兜底文案的存在，保证按码展示的地方也不是空串。
	if apperr.Message(apperr.CodeUserMuted) == "" {
		t.Fatal("user.muted 必须有兜底文案")
	}
}

func TestNormalizeMuteDays(t *testing.T) {
	cases := []struct {
		name string
		raw  int
		want int
		code apperr.Code
	}{
		{"未传按默认 7 天", 0, domain.DefaultMuteDays, ""},
		{"下限 1 天", 1, 1, ""},
		{"上限 365 天", 365, 365, ""},
		{"负数越界", -1, 0, apperr.CodeUserMuteDaysInvalid},
		{"超过 365 越界", 366, 0, apperr.CodeUserMuteDaysInvalid},
	}
	for _, c := range cases {
		got, err := domain.NormalizeMuteDays(c.raw)
		assertCode(t, err, c.code)
		if c.code == "" && got != c.want {
			t.Fatalf("%s: NormalizeMuteDays(%d)=%d，期望 %d", c.name, c.raw, got, c.want)
		}
	}
}

// TestMuteUntilAt 钉住"天数 → 到期时间"的换算（按整天加，不做小时取整）。
func TestMuteUntilAt(t *testing.T) {
	got := domain.MuteUntilAt(now, 7)
	if want := now.Add(7 * 24 * time.Hour); !got.Equal(want) {
		t.Fatalf("7 天禁言应到 %s，实际 %s", want, got)
	}
}
