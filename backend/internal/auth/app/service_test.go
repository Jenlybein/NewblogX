package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/app"
	"myblogx/internal/auth/apptest"
	"myblogx/internal/auth/domain"
	"myblogx/internal/kernel/ctype"
)

var baseTime = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

const (
	aliceID = ctype.ID("1001")
	alicePw = "secret123"
)

func setup(t *testing.T) *apptest.Repos {
	t.Helper()
	r := apptest.NewRepos(baseTime)
	r.Credentials().Seed(app.Account{
		ID: aliceID, Username: "alice", Role: 2, TokenVersion: 1,
	}, alicePw)
	return r
}

func meta() domain.Meta {
	return domain.Meta{IP: "1.2.3.4", Addr: "杭州", UA: "test-agent"}
}

func login(t *testing.T, r *apptest.Repos) *app.LoginResult {
	t.Helper()
	res, err := r.Service().Login(context.Background(), app.LoginInput{
		Account: "alice", Password: alicePw, Meta: meta(),
	})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	return res
}

func assertCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
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

func TestLogin(t *testing.T) {
	t.Run("成功签发双令牌并建会话", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)

		if res.Tokens.Access == "" || res.Tokens.Refresh == "" {
			t.Fatal("双令牌不能为空")
		}
		if res.Session.ID.IsZero() {
			t.Fatal("会话 ID 未回填")
		}
		if r.Sessions().Count() != 1 {
			t.Fatalf("应建 1 条会话，实际 %d", r.Sessions().Count())
		}
		if res.Session.ExpiresAt.Sub(baseTime) != 30*24*time.Hour {
			t.Fatalf("会话有效期不对: %v", res.Session.ExpiresAt.Sub(baseTime))
		}
	})

	t.Run("管理入口拒绝非管理员且不建会话", func(t *testing.T) {
		t.Parallel()
		r := setup(t) // alice 的 Role = 2（普通用户）
		_, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "alice", Password: alicePw, Meta: meta(),
			RequireAdmin: true,
		})
		assertCode(t, err, apperr.CodeAuthAdminRequired)
		// 关键：拒绝必须发生在 startSession 之前。
		// 否则库里会留下一条孤儿会话、refresh cookie 也已经写出去 ——
		// 那等于"告诉你没权限，同时还发了一张票"。
		if r.Sessions().Count() != 0 {
			t.Fatalf("被拒的管理登录不该建会话，实际 %d 条", r.Sessions().Count())
		}
	})

	t.Run("管理入口放行管理员", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		const rootPw = "root-secret"
		r.Credentials().Seed(app.Account{
			ID: ctype.ID("1002"), Username: "root", Role: 1, TokenVersion: 1, // 1 = 管理员
		}, rootPw)
		res, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "root", Password: rootPw, Meta: meta(),
			RequireAdmin: true,
		})
		if err != nil {
			t.Fatalf("管理员应能登录: %v", err)
		}
		if res.Tokens.Access == "" || r.Sessions().Count() != 1 {
			t.Fatal("管理员登录应正常签发令牌并建会话")
		}
	})

	t.Run("读者入口不校验角色", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		// 同一个普通账号，不传 scope 时必须照旧登录成功 ——
		// 否则"后台加门禁"会顺手把读者站的登录弄坏。
		if res := login(t, r); res.Tokens.Access == "" {
			t.Fatal("读者入口不应受 scope 影响")
		}
	})

	t.Run("密码错时记失败且不建会话", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "alice", Password: "wrong", Meta: meta(),
		})
		assertCode(t, err, apperr.CodeUserInvalidCredential)
		if r.Guard().Failures() != 1 {
			t.Fatalf("应记 1 次失败，实际 %d", r.Guard().Failures())
		}
		if r.Sessions().Count() != 0 {
			t.Fatal("登录失败不该建会话")
		}
	})

	t.Run("被限流时直接拒绝且不验密码", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		r.Guard().Block(true)
		_, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "alice", Password: alicePw, Meta: meta(),
		})
		assertCode(t, err, apperr.CodeAuthTooFrequent)
		if r.Guard().Failures() != 0 {
			t.Fatal("被限流时不该再累加失败次数")
		}
	})

	t.Run("限流器故障时放行", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		r.Guard().FailWith(errors.New("redis down"))
		res, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "alice", Password: alicePw, Meta: meta(),
		})
		if err != nil {
			t.Fatalf("限流器故障不该阻断登录: %v", err)
		}
		if res.Tokens.Access == "" {
			t.Fatal("应正常签发令牌")
		}
	})

	t.Run("登录成功清零失败计数", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		login(t, r)
		if r.Guard().Resets() != 1 {
			t.Fatalf("应清零 1 次，实际 %d", r.Guard().Resets())
		}
	})

	t.Run("签发令牌失败时要回滚会话", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		r.Issuer().FailWith(errors.New("sign failed"))
		_, err := r.Service().Login(context.Background(), app.LoginInput{
			Account: "alice", Password: alicePw, Meta: meta(),
		})
		if err == nil {
			t.Fatal("签发失败应返回错误")
		}
		if r.Sessions().Revokes() != 1 {
			t.Fatalf("应吊销刚建的会话，实际吊销 %d 次", r.Sessions().Revokes())
		}
	})

	t.Run("空密码直接拒", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Login(context.Background(), app.LoginInput{Account: "alice", Meta: meta()})
		assertCode(t, err, apperr.CodeUserInvalidCredential)
	})
}

func TestRefresh(t *testing.T) {
	t.Run("轮换后旧刷新令牌失效", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := login(t, r)

		second, err := r.Service().Refresh(context.Background(), first.Tokens.Refresh, meta())
		if err != nil {
			t.Fatalf("刷新失败: %v", err)
		}
		if second.Tokens.Refresh == first.Tokens.Refresh {
			t.Fatal("刷新令牌必须轮换")
		}
		if second.Session.ID != first.Session.ID {
			t.Fatal("轮换不该换会话 ID")
		}
		if r.Sessions().Rotates() != 1 {
			t.Fatalf("应轮换 1 次，实际 %d", r.Sessions().Rotates())
		}

		// 旧令牌再用来换，必须失败。
		if _, err := r.Service().Refresh(context.Background(), first.Tokens.Refresh, meta()); err == nil {
			t.Fatal("旧刷新令牌应已失效")
		}
	})

	t.Run("不存在的刷新令牌", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Refresh(context.Background(), "not-exist", meta())
		assertCode(t, err, apperr.CodeAuthRefreshInvalid)
	})

	t.Run("会话过期后不能刷新", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := login(t, r)
		r.Clock().Advance(31 * 24 * time.Hour)
		_, err := r.Service().Refresh(context.Background(), first.Tokens.Refresh, meta())
		assertCode(t, err, apperr.CodeAuthRefreshInvalid)
	})

	t.Run("空刷新令牌", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Refresh(context.Background(), "", meta())
		assertCode(t, err, apperr.CodeAuthRefreshInvalid)
	})

	t.Run("账号版本被提升后刷新仍拿到正确版本", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := login(t, r)
		// 模拟改密码 / 踢下线：账号版本 +1。
		r.Credentials().Seed(app.Account{ID: aliceID, Username: "alice", Role: 2, TokenVersion: 2}, alicePw)

		second, err := r.Service().Refresh(context.Background(), first.Tokens.Refresh, meta())
		if err != nil {
			t.Fatalf("刷新失败: %v", err)
		}
		if got := tokenVersionOf(t, r, second.Tokens.Access); got != 2 {
			t.Fatalf("刷新签发的令牌版本应等于账号当前版本: got=%d want=2", got)
		}
		// 端到端：新令牌必须立刻能用，不能被判成"状态已过期"。
		if _, err := r.Service().Authenticate(context.Background(), second.Tokens.Access); err != nil {
			t.Fatalf("刷新后的令牌应可直接通过鉴权: %v", err)
		}
	})

	t.Run("读到陈旧账号副本时也要签发当前版本", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := login(t, r)
		r.Credentials().Seed(app.Account{ID: aliceID, Username: "alice", Role: 2, TokenVersion: 2}, alicePw)
		// 这一次 FindAccount 会返回陈旧副本（版本 1）。
		r.Credentials().StaleFindAccount(aliceID, 1)

		second, err := r.Service().Refresh(context.Background(), first.Tokens.Refresh, meta())
		if err != nil {
			t.Fatalf("刷新失败: %v", err)
		}
		if got := tokenVersionOf(t, r, second.Tokens.Access); got != 2 {
			t.Fatalf("读到陈旧副本时也应签发当前版本 2: got=%d", got)
		}
		if _, err := r.Service().Authenticate(context.Background(), second.Tokens.Access); err != nil {
			t.Fatalf("刷新后的令牌应可直接通过鉴权: %v", err)
		}
	})
}

// tokenVersionOf 解析访问令牌里的版本号，作为断言的观测点。
func tokenVersionOf(t *testing.T, r *apptest.Repos, access string) uint32 {
	t.Helper()
	claims, err := r.Issuer().Parse(access)
	if err != nil {
		t.Fatalf("解析令牌失败: %v", err)
	}
	return claims.TokenVersion
}

func TestAuthenticate(t *testing.T) {
	t.Run("正常令牌可以通过", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)
		p, err := r.Service().Authenticate(context.Background(), res.Tokens.Access)
		if err != nil {
			t.Fatalf("鉴权失败: %v", err)
		}
		if p.UserID != aliceID || p.SessionID.Uint64() != res.Session.ID.Uint64() {
			t.Fatalf("操作者信息不对: %+v", p)
		}
	})

	t.Run("空令牌", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Authenticate(context.Background(), "")
		assertCode(t, err, apperr.CodeAuthRequired)
	})

	t.Run("格式错误的令牌", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().Authenticate(context.Background(), "garbage")
		assertCode(t, err, apperr.CodeUserTokenInvalid)
	})

	t.Run("令牌版本不一致（改过密码）", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)
		// 模拟改密码后版本号 +1。
		r.Credentials().Seed(app.Account{ID: aliceID, Username: "alice", Role: 2, TokenVersion: 2}, alicePw)
		_, err := r.Service().Authenticate(context.Background(), res.Tokens.Access)
		assertCode(t, err, apperr.CodeUserSessionExpired)
	})

	t.Run("会话被吊销后令牌失效", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)
		if err := r.Service().RevokeAll(context.Background(), aliceID); err != nil {
			t.Fatalf("吊销失败: %v", err)
		}
		_, err := r.Service().Authenticate(context.Background(), res.Tokens.Access)
		assertCode(t, err, apperr.CodeUserSessionExpired)
	})

	t.Run("黑名单查询失败时按已注销处理", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)
		r.Blacklist().FailWith(errors.New("redis down"))
		_, err := r.Service().Authenticate(context.Background(), res.Tokens.Access)
		assertCode(t, err, apperr.CodeUserSessionExpired)
	})
}

func TestLogout(t *testing.T) {
	t.Run("注销会吊销会话并把访问令牌拉黑", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		res := login(t, r)
		p, err := r.Service().Authenticate(context.Background(), res.Tokens.Access)
		if err != nil {
			t.Fatalf("鉴权失败: %v", err)
		}

		if err := r.Service().Logout(context.Background(), p, res.Tokens.Access); err != nil {
			t.Fatalf("注销失败: %v", err)
		}
		if r.Sessions().Revokes() != 1 {
			t.Fatalf("应吊销 1 次，实际 %d", r.Sessions().Revokes())
		}
		if r.Blacklist().Size() != 1 {
			t.Fatalf("访问令牌应进黑名单，实际 %d 条", r.Blacklist().Size())
		}

		// 注销后同一个访问令牌必须失效。
		_, err = r.Service().Authenticate(context.Background(), res.Tokens.Access)
		assertCode(t, err, apperr.CodeUserSessionExpired)
	})

	t.Run("未登录时注销报错", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		err := r.Service().Logout(context.Background(), nil, "")
		assertCode(t, err, apperr.CodeAuthRequired)
	})
}

func TestRevokeAllAndList(t *testing.T) {
	t.Run("改密码后所有会话都失效", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		first := login(t, r)
		second := loginAs(t, r, domain.Meta{IP: "1.2.3.4", Addr: "杭州", UA: testUAFirefoxLinux})
		if got := len(mustList(t, r, ctype.ID(""))); got != 2 {
			t.Fatalf("应有 2 台有效设备，实际 %d", got)
		}

		if err := r.Service().RevokeAll(context.Background(), aliceID); err != nil {
			t.Fatalf("吊销失败: %v", err)
		}
		if got := len(mustList(t, r, ctype.ID(""))); got != 0 {
			t.Fatalf("吊销后应为 0 台，实际 %d", got)
		}
		for _, token := range []string{first.Tokens.Access, second.Tokens.Access} {
			if _, err := r.Service().Authenticate(context.Background(), token); err == nil {
				t.Fatal("吊销后旧令牌还能用")
			}
		}
	})

	t.Run("空用户 ID 不查列表", func(t *testing.T) {
		t.Parallel()
		r := setup(t)
		_, err := r.Service().ListSessions(context.Background(), ctype.NewID(0), ctype.ID(""))
		assertCode(t, err, apperr.CodeAuthRequired)
	})
}

func mustList(t *testing.T, r *apptest.Repos, current ctype.ID) []domain.LoginDevice {
	t.Helper()
	list, err := r.Service().ListSessions(context.Background(), aliceID, current)
	if err != nil {
		t.Fatalf("查会话列表失败: %v", err)
	}
	return list
}
