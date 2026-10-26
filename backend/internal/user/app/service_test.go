package app_test

import (
	"context"
	"errors"
	"myblogx/internal/kernel/ctype"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

var (
	baseTime = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	aliceID  = ctype.NewID(1001)
	adminID  = ctype.NewID(9001)
	alice    = domain.Viewer{ID: aliceID, Role: domain.RoleUser}
	admin    = domain.Viewer{ID: adminID, Role: domain.RoleAdmin}
)

func seed(t *testing.T, r *apptest.Repos) domain.User {
	t.Helper()
	u := r.Users().Seed(domain.User{
		ID:           aliceID,
		Username:     "alice",
		Nickname:     "爱丽丝",
		Avatar:       "https://cdn/a.png",
		Abstract:     "hi",
		Status:       domain.StatusActive,
		Role:         domain.RoleUser,
		PasswordHash: "hash:x",
		CreatedAt:    baseTime.AddDate(-3, 0, 0),
	})
	if err := r.Settings().InitDefaults(testCtx(), aliceID); err != nil {
		t.Fatalf("初始化偏好失败: %v", err)
	}
	if err := r.Stats().Init(testCtx(), aliceID); err != nil {
		t.Fatalf("初始化统计失败: %v", err)
	}
	return u
}

func TestDetail(t *testing.T) {
	t.Run("未登录不能看详情", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().Detail(testCtx(), domain.Anonymous())
		assertCode(t, err, apperr.CodeUserForbidden)
	})

	t.Run("正常返回并且字段齐备", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		r.Enricher().TagTitleMap[ctype.NewID(7)] = "Go"
		_ = r.Settings().Save(testCtx(), domain.Settings{
			UserID: aliceID, LikeTagIDs: []ctype.ID{ctype.NewID(7)}, FavoritesVisibility: true,
		})

		view, err := r.Service().Detail(testCtx(), alice)
		if err != nil {
			t.Fatalf("详情失败: %v", err)
		}
		if view.Username != "alice" || view.Nickname != "爱丽丝" {
			t.Fatalf("字段不对: %+v", view)
		}
		if view.CodeAge != 3 {
			t.Fatalf("CodeAge=%d want=3", view.CodeAge)
		}
		if !view.HasPassword {
			t.Fatal("有密码哈希时 HasPassword 应为 true")
		}
		if len(view.LikeTagItems) != 1 || view.LikeTagItems[0].Title != "Go" {
			t.Fatalf("标签项不对: %+v", view.LikeTagItems)
		}
	})

	t.Run("头像与标签查不到时降级但不报错", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		_ = r.Settings().Save(testCtx(), domain.Settings{UserID: aliceID, LikeTagIDs: []ctype.ID{ctype.NewID(404)}})

		view, err := r.Service().Detail(testCtx(), alice)
		if err != nil {
			t.Fatalf("降级场景不该失败: %v", err)
		}
		if view.AvatarImageID != nil {
			t.Fatal("查不到头像 ID 时不该返回值")
		}
		if len(view.LikeTagItems) != 0 {
			t.Fatal("查不到标签标题时应该是空数组")
		}
	})
}

func TestPublicProfile(t *testing.T) {
	t.Run("不存在的用户", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().PublicProfile(testCtx(), ctype.NewID(404))
		assertCode(t, err, apperr.CodeUserNotFound)
	})

	t.Run("被禁用的用户主页仍可看", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{ID: ctype.NewID(2002), Username: "bob", Status: domain.StatusBanned})
		profile, err := r.Service().PublicProfile(testCtx(), ctype.NewID(2002))
		if err != nil {
			t.Fatalf("封禁用户主页应可看: %v", err)
		}
		if profile.Username != "bob" {
			t.Fatalf("字段不对: %+v", profile)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("别人改不了我的资料", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		nick := "hacked"
		_, err := r.Service().Update(testCtx(), domain.Viewer{ID: ctype.NewID(2002), Role: domain.RoleUser}, aliceID,
			domain.ProfileUpdate{Nickname: &nick})
		assertCode(t, err, apperr.CodeUserForbidden)
	})

	t.Run("昵称为空被拒", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		empty := "  "
		_, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Nickname: &empty})
		assertCode(t, err, apperr.CodeUserNicknameRequired)
	})

	t.Run("新用户名被占用", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		r.Users().Seed(domain.User{ID: ctype.NewID(3003), Username: "taken"})
		next := "taken"
		_, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Username: &next})
		assertCode(t, err, apperr.CodeUserUsernameTaken)
	})

	t.Run("用户名冷却期内被拒", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		recent := baseTime.Add(-24 * time.Hour)
		_ = r.Settings().Save(testCtx(), domain.Settings{UserID: aliceID, UpdatedUsernameAt: &recent})

		next := "alice2"
		_, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Username: &next})
		assertCode(t, err, apperr.CodeUserUsernameCooldown)
	})

	t.Run("过了冷却期可以改并写入时间戳", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		old := baseTime.Add(-40 * 24 * time.Hour)
		_ = r.Settings().Save(testCtx(), domain.Settings{UserID: aliceID, UpdatedUsernameAt: &old})

		next := "alice2"
		if _, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Username: &next}); err != nil {
			t.Fatalf("应允许改名: %v", err)
		}
		calls := r.Users().ProfileCalls()
		if len(calls) != 1 || calls[0].At == nil {
			t.Fatalf("应写入冷却时间戳: %+v", calls)
		}
	})

	t.Run("改昵称会触发展示快照同步", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		nick := "新昵称"
		res, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Nickname: &nick})
		if err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		if !res.SnapshotSynced {
			t.Fatal("改昵称应触发快照同步")
		}
		if got := r.Snapshots().Calls(); len(got) != 1 || got[0] != aliceID {
			t.Fatalf("快照同步调用不对: %v", got)
		}
	})

	t.Run("只改用户名不触发快照同步", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		next := "alice3"
		res, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Username: &next})
		if err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		if res.SnapshotSynced || len(r.Snapshots().Calls()) != 0 {
			t.Fatal("用户名不在冗余展示字段里，不该触发同步")
		}
	})

	t.Run("快照同步失败不阻断主流程", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		r.Snapshots().FailWith(errors.New("db down"))
		nick := "另一个昵称"
		res, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Nickname: &nick})
		if err != nil {
			t.Fatalf("快照失败不该让资料更新失败: %v", err)
		}
		if res.SnapshotSynced {
			t.Fatal("同步失败时不该标记为已同步")
		}
	})

	t.Run("没有实际变更时不写库", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		same := "爱丽丝"
		if _, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Nickname: &same}); err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		if calls := r.Users().ProfileCalls(); len(calls) != 0 {
			t.Fatalf("无变更不该写库: %+v", calls)
		}
	})
}

func TestUpdateSettings(t *testing.T) {
	t.Run("可以关掉通知（false 是合法值）", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		off := false
		if err := r.Service().UpdateSettings(testCtx(), alice, aliceID, app.SettingsPatch{DiggNoticeEnabled: &off}); err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		got, _ := r.Settings().Find(testCtx(), aliceID)
		if got.DiggNoticeEnabled {
			t.Fatal("点赞通知应为已关闭")
		}
	})

	t.Run("别人改不了我的设置", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		off := false
		err := r.Service().UpdateSettings(testCtx(), domain.Viewer{ID: ctype.NewID(2002), Role: domain.RoleUser}, aliceID,
			app.SettingsPatch{DiggNoticeEnabled: &off})
		assertCode(t, err, apperr.CodeUserForbidden)
	})

	t.Run("空补丁不写库", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		before := r.Settings().Saves()
		if err := r.Service().UpdateSettings(testCtx(), alice, aliceID, app.SettingsPatch{}); err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		if r.Settings().Saves() != before {
			t.Fatal("空补丁不该写库")
		}
	})
}

func TestListAndChangeStatus(t *testing.T) {
	t.Run("普通用户不能查列表", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().List(testCtx(), alice, domain.Query{})
		assertCode(t, err, apperr.CodeUserForbidden)
	})

	t.Run("管理员可以查列表", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		page, err := r.Service().List(testCtx(), admin, domain.Query{})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if page.Total != 1 {
			t.Fatalf("总数不对: %d", page.Total)
		}
	})

	t.Run("管理员不能改自己的状态", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{ID: adminID, Username: "root", Role: domain.RoleAdmin, Status: domain.StatusActive})
		err := r.Service().ChangeStatus(testCtx(), admin, adminID, domain.StatusBanned)
		assertCode(t, err, apperr.CodeUserStatusSelfLocked)
	})

	t.Run("非法状态被拒", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		err := r.Service().ChangeStatus(testCtx(), admin, aliceID, domain.Status(99))
		assertCode(t, err, apperr.CodeUserStatusInvalid)
	})

	t.Run("状态相同时不写库", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		r.Users().FailNext(errors.New("不该被调用"))
		if err := r.Service().ChangeStatus(testCtx(), admin, aliceID, domain.StatusActive); err != nil {
			t.Fatalf("相同状态应直接返回: %v", err)
		}
	})
}

func TestCreate(t *testing.T) {
	t.Run("普通用户不能创建", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().Create(testCtx(), alice, app.CreateInput{Username: "x", Nickname: "y", Password: "p"})
		assertCode(t, err, apperr.CodeUserForbidden)
	})

	t.Run("成功创建时三张表都建档", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		u, err := r.Service().Create(testCtx(), admin, app.CreateInput{
			Username: "newbie", Nickname: "新人", Password: "secret123",
		})
		if err != nil {
			t.Fatalf("创建失败: %v", err)
		}
		if u.ID.IsZero() {
			t.Fatal("ID 未回填")
		}
		if u.RegisterSource != domain.SourceAdmin {
			t.Fatalf("注册来源应为管理员创建，实际: %v", u.RegisterSource)
		}
		if u.PasswordHash != "hash:secret123" {
			t.Fatalf("密码未哈希: %q", u.PasswordHash)
		}
		if _, err := r.Settings().Find(testCtx(), u.ID); err != nil {
			t.Fatalf("偏好未建档: %v", err)
		}
		if _, err := r.Stats().Find(testCtx(), u.ID); err != nil {
			t.Fatalf("统计未建档: %v", err)
		}
	})

	t.Run("密码为空被拒", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().Create(testCtx(), admin, app.CreateInput{Username: "nb", Nickname: "新人"})
		assertCode(t, err, apperr.CodeUserPasswordRequired)
	})

	t.Run("邮箱重复被拒", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		email := "dup@example.com"
		r.Users().Seed(domain.User{ID: ctype.NewID(5005), Username: "old", Email: &email})
		_, err := r.Service().Create(testCtx(), admin, app.CreateInput{
			Username: "newbie", Nickname: "新人", Password: "p", Email: &email,
		})
		assertCode(t, err, apperr.CodeUserEmailTaken)
	})
}

func TestStatsAndDisplay(t *testing.T) {
	t.Run("统计按增量累加", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		if err := r.Service().AddStats(testCtx(), aliceID, domain.StatsDelta{Article: 2, View: 5}); err != nil {
			t.Fatalf("累加失败: %v", err)
		}
		if err := r.Service().AddStats(testCtx(), aliceID, domain.StatsDelta{Article: 1}); err != nil {
			t.Fatalf("累加失败: %v", err)
		}
		got, _ := r.Stats().Find(testCtx(), aliceID)
		if got.ArticleCount != 3 || got.ViewCount != 5 {
			t.Fatalf("累加结果不对: %+v", got)
		}
	})

	t.Run("零增量不写库", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		if err := r.Service().AddStats(testCtx(), aliceID, domain.StatsDelta{}); err != nil {
			t.Fatalf("不该失败: %v", err)
		}
		if len(r.Stats().Deltas()) != 0 {
			t.Fatal("零增量不该写库")
		}
	})

	t.Run("批量读展示快照只返回存在的用户", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		seed(t, r)
		snaps, err := r.Service().LoadDisplays(testCtx(), []domain.UserID{aliceID, ctype.NewID(404)})
		if err != nil {
			t.Fatalf("读取失败: %v", err)
		}
		if len(snaps) != 1 || snaps[aliceID].Nickname != "爱丽丝" {
			t.Fatalf("结果不对: %+v", snaps)
		}
	})

	t.Run("禁用账号 EnsureUsable 报错", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{ID: ctype.NewID(6006), Username: "dis", Status: domain.StatusDisabled})
		assertCode(t, r.Service().EnsureUsable(testCtx(), ctype.NewID(6006)), apperr.CodeUserAccountDisabled)
	})
}

func testCtx() context.Context { return context.Background() }

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
