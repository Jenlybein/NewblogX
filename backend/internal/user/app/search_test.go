package app_test

import (
	"context"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// stubAuthorStats 是 AuthorStatsReader 的替身：既能断言"按哪个作者查"，
// 也能模拟"文章模块读挂了"。
type stubAuthorStats struct {
	got   domain.UserID
	stats app.AuthorStats
	err   error
	calls int
}

func (s *stubAuthorStats) AuthorStats(_ context.Context, id domain.UserID) (app.AuthorStats, error) {
	s.calls++
	s.got = id
	if s.err != nil {
		return app.AuthorStats{}, s.err
	}
	return s.stats, nil
}

func serviceWithAuthorStats(r *apptest.Repos, port app.AuthorStatsReader) *app.Service {
	return app.New(
		r.Users(),
		r.Settings(),
		r.Stats(),
		r.Snapshots(),
		nil,
		r.Clock(),
		apptest.Hasher{Prefix: "hash:"},
		nil,
		app.WithIntegrations(app.Integrations{AuthorStats: port}),
	)
}

func TestSearchUsers(t *testing.T) {
	t.Run("只回正常状态、只匹配昵称或用户名", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{
			ID: ctype.NewID(3001), Username: "jelly", Nickname: "JellyBin",
			Status: domain.StatusActive, CreatedAt: baseTime.AddDate(-2, 0, 0),
		})
		r.Users().Seed(domain.User{
			ID: ctype.NewID(3002), Username: "jelly_banned", Nickname: "JellyBin 小号",
			Status: domain.StatusBanned,
		})
		r.Users().Seed(domain.User{
			ID: ctype.NewID(3003), Username: "alice", Nickname: "爱丽丝",
			Status: domain.StatusActive,
		})

		items, total, err := r.Service().SearchUsers(testCtx(), "jelly", 1, 10)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if total != 1 || len(items) != 1 {
			t.Fatalf("应只命中 1 个正常账号，实际 total=%d items=%d", total, len(items))
		}
		got := items[0]
		if got.ID != ctype.NewID(3001) || got.Nickname != "JellyBin" {
			t.Fatalf("命中结果不对: %+v", got)
		}
		if got.CodeAge != 2 {
			t.Fatalf("注册年数应为 2，实际 %d", got.CodeAge)
		}
	})

	t.Run("空关键词不列全站用户", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{ID: ctype.NewID(3004), Username: "a", Nickname: "A", Status: domain.StatusActive})

		items, total, err := r.Service().SearchUsers(testCtx(), "   ", 1, 10)
		if err != nil {
			t.Fatalf("空关键词不该报错: %v", err)
		}
		if total != 0 || len(items) != 0 {
			t.Fatalf("空关键词应返回空列表，实际 total=%d items=%d", total, len(items))
		}
	})

	t.Run("分页按 limit 截断，total 仍是命中总数", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		for i := 0; i < 3; i++ {
			r.Users().Seed(domain.User{
				ID: ctype.NewID(uint64(4000 + i)), Username: "fan", Nickname: "粉丝",
				Status: domain.StatusActive,
			})
		}

		items, total, err := r.Service().SearchUsers(testCtx(), "fan", 1, 2)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if total != 3 {
			t.Fatalf("total 应为 3，实际 %d", total)
		}
		if len(items) != 2 {
			t.Fatalf("第一页应为 2 条，实际 %d", len(items))
		}

		second, total2, err := r.Service().SearchUsers(testCtx(), "fan", 2, 2)
		if err != nil {
			t.Fatalf("第二页搜索失败: %v", err)
		}
		if total2 != 3 || len(second) != 1 {
			t.Fatalf("第二页应为 1 条且 total=3，实际 total=%d items=%d", total2, len(second))
		}
	})
}

func TestPublicProfile_累计互动(t *testing.T) {
	t.Run("注入端口后带回三个累计数", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		port := &stubAuthorStats{stats: app.AuthorStats{Digg: 12, Favor: 7, Comment: 3}}
		svc := serviceWithAuthorStats(r, port)

		profile, err := svc.PublicProfile(testCtx(), aliceID)
		if err != nil {
			t.Fatalf("读取公开主页失败: %v", err)
		}
		if port.got != aliceID {
			t.Fatalf("应按作者 ID 查询，实际 %v", port.got)
		}
		if profile.DiggCount != 12 || profile.FavorCount != 7 || profile.CommentCount != 3 {
			t.Fatalf("累计数不对: %+v", profile)
		}
	})

	t.Run("文章模块读失败时主页照常打开，三个数为 0", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		port := &stubAuthorStats{err: context.DeadlineExceeded}
		svc := serviceWithAuthorStats(r, port)

		profile, err := svc.PublicProfile(testCtx(), aliceID)
		if err != nil {
			t.Fatalf("聚合失败不该让主页打不开: %v", err)
		}
		if profile.DiggCount != 0 || profile.FavorCount != 0 || profile.CommentCount != 0 {
			t.Fatalf("失败时应保持 0，实际 %+v", profile)
		}
		if profile.Nickname == "" {
			t.Fatal("其它字段仍应正常返回")
		}
	})
}
