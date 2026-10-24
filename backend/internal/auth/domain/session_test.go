package domain_test

import (
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/domain"
)

// 会话有效性是个三条件与：未吊销 × 未过期 × 有 ID。
// 少判任何一个都会留下安全漏洞，所以这里穷举。

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func revoked(at time.Time) *time.Time { return &at }

func TestSessionIsActive(t *testing.T) {
	cases := []struct {
		name    string
		session domain.Session
		want    bool
	}{
		{"正常会话", domain.Session{ID: 1, ExpiresAt: now.Add(time.Hour)}, true},
		{"已吊销", domain.Session{ID: 1, ExpiresAt: now.Add(time.Hour), RevokedAt: revoked(now.Add(-time.Minute))}, false},
		{"已过期", domain.Session{ID: 1, ExpiresAt: now.Add(-time.Second)}, false},
		{"恰好到期（不算有效）", domain.Session{ID: 1, ExpiresAt: now}, false},
		{"已吊销且已过期", domain.Session{ID: 1, ExpiresAt: now.Add(-time.Hour), RevokedAt: revoked(now)}, false},
		{"零值会话", domain.Session{}, false},
		{"无 ID 但有过期时间", domain.Session{ExpiresAt: now.Add(time.Hour)}, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.session.IsActive(now); got != c.want {
				t.Fatalf("IsActive got=%v want=%v", got, c.want)
			}
			if got := c.session.CanRefresh(now); got != c.want {
				t.Fatalf("CanRefresh 应与 IsActive 一致 got=%v want=%v", got, c.want)
			}
		})
	}
}

func TestSessionEnsureActive(t *testing.T) {
	active := domain.Session{ID: 1, ExpiresAt: now.Add(time.Hour)}
	if err := active.EnsureActive(now); err != nil {
		t.Fatalf("有效会话不该报错: %v", err)
	}

	expired := domain.Session{ID: 1, ExpiresAt: now.Add(-time.Hour)}
	err := expired.EnsureActive(now)
	if err == nil {
		t.Fatal("过期会话必须报错")
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != apperr.CodeAuthRefreshInvalid {
		t.Fatalf("错误码不对: %v", err)
	}
}

func TestSessionExpiredBy(t *testing.T) {
	// ExpiredBy 只看时间，不看吊销：清理任务要留审计期。
	revokedButFuture := domain.Session{ID: 1, ExpiresAt: now.Add(time.Hour), RevokedAt: revoked(now)}
	if revokedButFuture.ExpiredBy(now) {
		t.Fatal("未到期但已吊销的会话不该被清理任务删掉")
	}
	past := domain.Session{ID: 1, ExpiresAt: now.Add(-time.Hour)}
	if !past.ExpiredBy(now) {
		t.Fatal("已过期会话应被判定为可清理")
	}
}

func TestValidateIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want apperr.Code
	}{
		{"alice", ""},
		{"alice@example.com", ""},
		{"", apperr.CodeUserInvalidCredential},
	}
	for _, c := range cases {
		err := domain.ValidateIdentifier(c.in)
		if c.want == "" {
			if err != nil {
				t.Fatalf("in=%q 不该报错: %v", c.in, err)
			}
			continue
		}
		code, _ := apperr.CodeOf(err)
		if code != c.want {
			t.Fatalf("in=%q got=%v want=%v", c.in, code, c.want)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want apperr.Code
	}{
		{"长度刚好达标", "12345678", ""},
		{"超长没问题", "1234567890123456789012345", ""},
		{"空密码", "", apperr.CodeUserPasswordRequired},
		{"太短", "1234567", apperr.CodeUserPasswordTooShort},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := domain.ValidatePassword(c.in)
			if c.want == "" {
				if err != nil {
					t.Fatalf("不该报错: %v", err)
				}
				return
			}
			code, _ := apperr.CodeOf(err)
			if code != c.want {
				t.Fatalf("got=%v want=%v", code, c.want)
			}
		})
	}
}
