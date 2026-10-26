package domain_test

import (
	"myblogx/internal/kernel/ctype"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// 本文件是用户模块的不变量矩阵：把"什么身份 × 什么状态下能做什么"穷举一遍。
//
// 为什么穷举而不是只测 happy path：权限判断的 bug 从来不是"该放行的没放行"
// （那个一测就发现），而是"该拦的没拦"——那种只有在反向用例里才暴露。

var (
	now       = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	aliceID   = ctype.NewID(1001)
	bobID     = ctype.NewID(1002)
	adminID   = ctype.NewID(9001)
	aliceView = domain.Viewer{ID: aliceID, Role: domain.RoleUser}
	bobView   = domain.Viewer{ID: bobID, Role: domain.RoleUser}
	adminView = domain.Viewer{ID: adminID, Role: domain.RoleAdmin}
	guestView = domain.Anonymous()
)

func activeUser(id domain.UserID, role domain.Role) domain.User {
	return domain.User{
		ID:        id,
		Username:  "u" + id.String(),
		Nickname:  "nick" + id.String(),
		Status:    domain.StatusActive,
		Role:      role,
		CreatedAt: now.AddDate(-2, 0, 0),
	}
}

func TestCanViewProfile(t *testing.T) {
	cases := []struct {
		name   string
		target domain.User
		wantOK bool
	}{
		{"存在的用户", activeUser(aliceID, domain.RoleUser), true},
		{"被禁用的用户主页仍可看", domain.User{ID: aliceID, Status: domain.StatusDisabled}, true},
		{"被封禁的用户主页仍可看", domain.User{ID: aliceID, Status: domain.StatusBanned}, true},
		{"零值用户（不存在）", domain.User{}, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := domain.CanViewProfile(c.target)
			if (err == nil) != c.wantOK {
				t.Fatalf("got err=%v wantOK=%v", err, c.wantOK)
			}
		})
	}
}

func TestCanUpdateProfile(t *testing.T) {
	target := activeUser(aliceID, domain.RoleUser)
	cases := []struct {
		name   string
		target domain.User
		viewer domain.Viewer
		want   apperr.Code // 空串表示期望成功
	}{
		{"本人改自己", target, aliceView, ""},
		{"管理员改别人", target, adminView, ""},
		{"别的普通用户改不了", target, bobView, apperr.CodeUserForbidden},
		{"未登录改不了", target, guestView, apperr.CodeUserForbidden},
		{"目标不存在", domain.User{}, adminView, apperr.CodeUserNotFound},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := domain.CanUpdateProfile(c.target, c.viewer)
			assertCode(t, err, c.want)
		})
	}
}

func TestCanChangeStatus(t *testing.T) {
	target := activeUser(aliceID, domain.RoleUser)
	self := activeUser(adminID, domain.RoleAdmin)
	cases := []struct {
		name   string
		target domain.User
		viewer domain.Viewer
		want   apperr.Code
	}{
		{"管理员禁用普通用户", target, adminView, ""},
		{"管理员封禁普通用户", target, adminView, ""},
		{"管理员解禁普通用户", target, adminView, ""},
		{"管理员不能改自己的状态", self, adminView, apperr.CodeUserStatusSelfLocked},
		{"普通用户不能封别人", target, bobView, apperr.CodeUserForbidden},
		{"普通用户不能解封自己", domain.User{ID: bobID, Status: domain.StatusBanned}, bobView, apperr.CodeUserForbidden},
		{"未登录不能改状态", target, guestView, apperr.CodeUserForbidden},
		{"目标不存在", domain.User{}, adminView, apperr.CodeUserNotFound},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, domain.CanChangeStatus(c.target, c.viewer), c.want)
		})
	}
}

func TestCanManageUsers(t *testing.T) {
	cases := []struct {
		name   string
		viewer domain.Viewer
		want   apperr.Code
	}{
		{"管理员可以", adminView, ""},
		{"普通用户不可以", aliceView, apperr.CodeUserForbidden},
		{"访客角色不可以", domain.Viewer{ID: bobID, Role: domain.RoleGuest}, apperr.CodeUserForbidden},
		{"未登录不可以", guestView, apperr.CodeUserForbidden},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, domain.CanManageUsers(c.viewer), c.want)
		})
	}
}

func TestCanLogin(t *testing.T) {
	cases := []struct {
		name   string
		status domain.Status
		want   apperr.Code
	}{
		{"正常可登录", domain.StatusActive, ""},
		{"禁用不可登录", domain.StatusDisabled, apperr.CodeUserAccountDisabled},
		{"封禁不可登录", domain.StatusBanned, apperr.CodeUserAccountBanned},
		{"未知状态不可登录", domain.Status(99), apperr.CodeUserStatusInvalid},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, domain.CanLogin(domain.User{Status: c.status}), c.want)
		})
	}
}

func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  apperr.Code
	}{
		{"合法小写", "alice_01", ""},
		{"合法大小写混合加连字符", "Alice-01", ""},
		{"空字符串", "", apperr.CodeUserUsernameInvalid},
		{"纯空格", "   ", apperr.CodeUserUsernameInvalid},
		{"超长", strings.Repeat("a", domain.MaxUsernameLen+1), apperr.CodeUserUsernameInvalid},
		{"含空格", "a b", apperr.CodeUserUsernameInvalid},
		{"含中文", "张三", apperr.CodeUserUsernameInvalid},
		{"含斜杠", "a/b", apperr.CodeUserUsernameInvalid},
		{"含 at 符号", "a@b", apperr.CodeUserUsernameInvalid},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, domain.ValidateUsername(c.input), c.want)
		})
	}
}

func TestValidateNicknameAndAbstract(t *testing.T) {
	cases := []struct {
		name string
		fn   func() error
		want apperr.Code
	}{
		{"昵称合法", func() error { return domain.ValidateNickname("小明") }, ""},
		{"昵称带首尾空格会被 trim 后通过", func() error { return domain.ValidateNickname("  小明  ") }, ""},
		{"昵称为空", func() error { return domain.ValidateNickname("") }, apperr.CodeUserNicknameRequired},
		{"昵称纯空格", func() error { return domain.ValidateNickname("   ") }, apperr.CodeUserNicknameRequired},
		{"昵称超长", func() error {
			return domain.ValidateNickname(strings.Repeat("字", domain.MaxNicknameLen+1))
		}, apperr.CodeUserNicknameTooLong},
		{"简介合法", func() error { return domain.ValidateAbstract("hello") }, ""},
		{"简介超长", func() error {
			return domain.ValidateAbstract(strings.Repeat("字", domain.MaxAbstractLen+1))
		}, apperr.CodeUserAbstractTooLong},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, c.fn(), c.want)
		})
	}
}

func TestCanUpdateUsername(t *testing.T) {
	target := activeUser(aliceID, domain.RoleUser)
	cases := []struct {
		name     string
		settings domain.Settings
		viewer   domain.Viewer
		want     apperr.Code
	}{
		{"从未改过可以改", domain.DefaultSettings(aliceID), aliceView, ""},
		{"29 天前改过，还在冷却期", settingsWithUsernameAge(aliceID, 29*24*time.Hour), aliceView, apperr.CodeUserUsernameCooldown},
		{"31 天前改过，已过冷却期", settingsWithUsernameAge(aliceID, 31*24*time.Hour), aliceView, ""},
		{"管理员也要等冷却期", settingsWithUsernameAge(aliceID, 1*24*time.Hour), adminView, apperr.CodeUserUsernameCooldown},
		{"别人不能帮他改", domain.DefaultSettings(aliceID), bobView, apperr.CodeUserForbidden},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, domain.CanUpdateUsername(target, c.settings, c.viewer, now), c.want)
		})
	}
}

func TestApplyProfileUpdate(t *testing.T) {
	base := activeUser(aliceID, domain.RoleUser)
	base.Abstract = "old"
	empty := ""
	newNick := "  new nick  "

	t.Run("只改昵称时返回 nickname 且做 trim", func(t *testing.T) {
		t.Parallel()
		u := base
		changed := u.ApplyProfileUpdate(domain.ProfileUpdate{Nickname: &newNick})
		if len(changed) != 1 || changed[0] != "nickname" {
			t.Fatalf("changed=%v", changed)
		}
		if u.Nickname != "new nick" {
			t.Fatalf("昵称未 trim: %q", u.Nickname)
		}
	})

	t.Run("显式传空字符串能清空简介", func(t *testing.T) {
		t.Parallel()
		u := base
		changed := u.ApplyProfileUpdate(domain.ProfileUpdate{Abstract: &empty})
		if len(changed) != 1 || changed[0] != "abstract" {
			t.Fatalf("changed=%v", changed)
		}
		if u.Abstract != "" {
			t.Fatalf("简介未清空: %q", u.Abstract)
		}
	})

	t.Run("值没变时不产生变更", func(t *testing.T) {
		t.Parallel()
		u := base
		same := base.Nickname
		if changed := u.ApplyProfileUpdate(domain.ProfileUpdate{Nickname: &same}); len(changed) != 0 {
			t.Fatalf("不该有变更: %v", changed)
		}
	})
}

func TestNeedsDisplaySnapshotSync(t *testing.T) {
	cases := []struct {
		changed []string
		want    bool
	}{
		{[]string{"nickname"}, true},
		{[]string{"avatar"}, true},
		{[]string{"abstract"}, true},
		{[]string{"username"}, false},
		{nil, false},
		{[]string{"username", "abstract"}, true},
	}
	for _, c := range cases {
		if got := domain.NeedsDisplaySnapshotSync(c.changed); got != c.want {
			t.Fatalf("changed=%v got=%v want=%v", c.changed, got, c.want)
		}
	}
}

func TestUserValueHelpers(t *testing.T) {
	u := activeUser(aliceID, domain.RoleUser)
	u.PasswordHash = ""
	u.TokenVersion = 3

	if !u.CreatedAt.Equal(now.AddDate(-2, 0, 0)) {
		t.Fatal("前置数据错误")
	}
	if got := u.CodeAge(now); got != 2 {
		t.Fatalf("CodeAge=%d want=2", got)
	}
	if u.HasPassword() {
		t.Fatal("密码为空时 HasPassword 应为 false")
	}
	if !u.TokenVersionMatches(3) {
		t.Fatal("token 版本应匹配")
	}
	if u.TokenVersionMatches(2) {
		t.Fatal("token 版本不匹配时必须拒绝")
	}
}

func TestProfileUpdateIsEmpty(t *testing.T) {
	if !(domain.ProfileUpdate{}).IsEmpty() {
		t.Fatal("空更新应被判定为空")
	}
	s := "x"
	if (domain.ProfileUpdate{Nickname: &s}).IsEmpty() {
		t.Fatal("有字段时不应判定为空")
	}
}

func TestQueryNormalize(t *testing.T) {
	cases := []struct {
		in       domain.Query
		wantPage int
		wantSize int
	}{
		{domain.Query{}, 1, 20},
		{domain.Query{Page: -1, Size: -5}, 1, 20},
		{domain.Query{Page: 3, Size: 50}, 3, 50},
		{domain.Query{Page: 1, Size: 500}, 1, 100},
	}
	for _, c := range cases {
		got := c.in.Normalize()
		if got.Page != c.wantPage || got.Size != c.wantSize {
			t.Fatalf("in=%+v got page=%d size=%d", c.in, got.Page, got.Size)
		}
		if got.Offset() != (c.wantPage-1)*c.wantSize {
			t.Fatalf("offset 计算错误: %d", got.Offset())
		}
	}
}

func settingsWithUsernameAge(id domain.UserID, age time.Duration) domain.Settings {
	s := domain.DefaultSettings(id)
	at := now.Add(-age)
	s.UpdatedUsernameAt = &at
	return s
}

func assertCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("期望成功，实际: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("期望错误码 %s，实际成功", want)
	}
	code, ok := apperr.CodeOf(err)
	if !ok {
		t.Fatalf("错误没有带错误码: %v", err)
	}
	if code != want {
		t.Fatalf("错误码 got=%s want=%s", code, want)
	}
}
