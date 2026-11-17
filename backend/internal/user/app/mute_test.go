package app_test

import (
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// 禁言用例（票 399 / ADR-0032 决策 8）：
//   - 只有管理员能设/解除，且不能操作自己；
//   - 时长 1–365、缺省 7；越界被拒；
//   - 被禁言者昵称 / 简介修改被拒（头像 / 用户名不受影响），管理员重置资料不受限；
//   - 到期后惰性放行，与每小时对账任务无关；
//   - 对账清空已到期行、不动未到期行、重跑幂等。

// mutedAlice 把 alice 置为"禁言到 now+days"，返回更新后的用户。
func mutedAlice(t *testing.T, r *apptest.Repos, days int) domain.User {
	t.Helper()
	until := baseTime.Add(time.Duration(days) * 24 * time.Hour)
	return r.Users().Seed(domain.User{
		ID:        aliceID,
		Username:  "alice",
		Nickname:  "爱丽丝",
		Status:    domain.StatusActive,
		Role:      domain.RoleUser,
		CreatedAt: baseTime.AddDate(-3, 0, 0),
		MuteUntil: &until,
	})
}

func TestMute_SetsUntilByDays(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)

	for _, tc := range []struct {
		name string
		days int
		want time.Duration
	}{
		{"缺省 7 天（传 0）", 0, 7 * 24 * time.Hour},
		{"指定 30 天", 30, 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Service().Mute(testCtx(), admin, aliceID, tc.days)
			if err != nil {
				t.Fatalf("管理员禁言不该失败: %v", err)
			}
			if got.MuteUntil == nil {
				t.Fatal("禁言后 MuteUntil 不能为空")
			}
			if want := baseTime.Add(tc.want); !got.MuteUntil.Equal(want) {
				t.Fatalf("到期时间应为 %s，实际 %s", want, got.MuteUntil)
			}
		})
	}
}

func TestMute_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)

	cases := []struct {
		name    string
		viewer  domain.Viewer
		days    int
		wantErr apperr.Code
	}{
		{"非管理员不能禁言", alice, 7, apperr.CodeUserForbidden},
		{"天数越界", admin, 366, apperr.CodeUserMuteDaysInvalid},
		{"天数为负", admin, -1, apperr.CodeUserMuteDaysInvalid},
	}
	for _, c := range cases {
		if _, err := r.Service().Mute(testCtx(), c.viewer, aliceID, c.days); !apperr.IsCode(err, c.wantErr) {
			t.Fatalf("%s: 期望错误码 %s，实际 %v", c.name, c.wantErr, err)
		}
	}
}

func TestMute_CannotMuteSelf(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	boss := r.Users().Seed(domain.User{
		ID: adminID, Username: "root", Nickname: "站长",
		Status: domain.StatusActive, Role: domain.RoleAdmin,
	})
	if _, err := r.Service().Mute(testCtx(), admin, boss.ID, 7); !apperr.IsCode(err, apperr.CodeUserStatusSelfLocked) {
		t.Fatalf("管理员不能禁言自己，实际 %v", err)
	}
}

func TestUnmute_ClearsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	mutedAlice(t, r, 7)

	if _, err := r.Service().Unmute(testCtx(), admin, aliceID); err != nil {
		t.Fatalf("解除禁言失败: %v", err)
	}
	got, err := r.Users().FindByID(testCtx(), aliceID)
	if err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}
	if got.MuteUntil != nil {
		t.Fatalf("解除禁言后 MuteUntil 应为 nil，实际 %v", got.MuteUntil)
	}
	// 幂等：本来就没禁言时再解一次也成功，不写库、不报错。
	if _, err := r.Service().Unmute(testCtx(), admin, aliceID); err != nil {
		t.Fatalf("重复解除禁言应幂等成功，实际 %v", err)
	}
	// 非管理员不能解除。
	if _, err := r.Service().Unmute(testCtx(), alice, aliceID); !apperr.IsCode(err, apperr.CodeUserForbidden) {
		t.Fatalf("普通用户不能解除禁言，实际 %v", err)
	}
}

func TestEnsureCanPublish(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	mutedAlice(t, r, 3)

	err := r.Service().EnsureCanPublish(testCtx(), aliceID)
	if !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("禁言未到期时 EnsureCanPublish 应返回 user.muted，实际 %v", err)
	}
	// 文案要带到期时间：前端直接把 err 的文案展示给用户。
	until := baseTime.Add(3 * 24 * time.Hour)
	if !strings.Contains(err.Error(), until.Format("2006-01-02 15:04")) {
		t.Fatalf("文案应含到期时间，实际 %q", err.Error())
	}

	// 登录 / 浏览不受影响：禁言不改账号状态，CanLogin 仍然通过。
	if err := r.Service().EnsureUsable(testCtx(), aliceID); err != nil {
		t.Fatalf("禁言不应让 EnsureUsable 失败（登录/浏览照旧）: %v", err)
	}

	// 到期后惰性放行：不需要跑对账任务，时间一到就能发。
	if err := r.Service().EnsureCanPublish(testCtx(), aliceID); !apperr.IsCode(err, apperr.CodeUserMuted) {
		t.Fatalf("未到期前应持续为 user.muted，实际 %v", err)
	}
	r.Clock().Advance(72*time.Hour + time.Minute)
	if err := r.Service().EnsureCanPublish(testCtx(), aliceID); err != nil {
		t.Fatalf("到期后应放行（惰性判定），实际 %v", err)
	}
}

func TestUpdateProfile_BlocksNicknameAndAbstractOnly(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	mutedAlice(t, r, 7)

	nickname := "新昵称"
	abstract := "新简介"
	for _, tc := range []struct {
		name   string
		update domain.ProfileUpdate
		want   apperr.Code
	}{
		{"改昵称被拒", domain.ProfileUpdate{Nickname: &nickname}, apperr.CodeUserMuted},
		{"改简介被拒", domain.ProfileUpdate{Abstract: &abstract}, apperr.CodeUserMuted},
	} {
		if _, err := r.Service().Update(testCtx(), alice, aliceID, tc.update); !apperr.IsCode(err, tc.want) {
			t.Fatalf("%s: 期望 %s，实际 %v", tc.name, tc.want, err)
		}
	}

	// 头像不在禁言范围：只改头像应当成功。
	avatar := "https://cdn/new.png"
	if _, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Avatar: &avatar}); err != nil {
		t.Fatalf("禁言不应拦住头像修改: %v", err)
	}

	// 管理员重置资料（风险页处置）走同一个 Update，不能被禁言挡住。
	problem := "问题昵称" + aliceID.String()
	empty := ""
	if _, err := r.Service().Update(testCtx(), admin, aliceID, domain.ProfileUpdate{Nickname: &problem, Abstract: &empty}); err != nil {
		t.Fatalf("管理员重置资料不应被禁言拦住: %v", err)
	}
	// 但被禁言者连管理员也不能直接改他的禁言状态以外的资料吗？不 —— 上面这条已经证明可以；
	// 普通用户改自己别的字段（用户名）不受影响。
	newName := "alice_new"
	if _, err := r.Service().Update(testCtx(), alice, aliceID, domain.ProfileUpdate{Username: &newName}); err != nil {
		t.Fatalf("禁言不应拦住用户名修改: %v", err)
	}
}

func TestClearExpiredMutes(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	expired := baseTime.Add(-time.Hour)
	future := baseTime.Add(48 * time.Hour)
	r.Users().Seed(domain.User{ID: ctype.NewID(2001), Username: "expired", Status: domain.StatusActive, MuteUntil: &expired})
	r.Users().Seed(domain.User{ID: ctype.NewID(2002), Username: "future", Status: domain.StatusActive, MuteUntil: &future})

	cleared, err := r.Service().ClearExpiredMutes(testCtx(), baseTime)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("应清空 1 个已到期账号，实际 %d", cleared)
	}
	got, err := r.Users().FindByID(testCtx(), ctype.NewID(2001))
	if err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}
	if got.MuteUntil != nil {
		t.Fatalf("已到期账号的禁言应被清空，实际 %v", got.MuteUntil)
	}
	keep, err := r.Users().FindByID(testCtx(), ctype.NewID(2002))
	if err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}
	if keep.MuteUntil == nil {
		t.Fatal("未到期账号的禁言不应被清空")
	}
	// 幂等：重跑一次清 0 个。
	again, err := r.Service().ClearExpiredMutes(testCtx(), baseTime)
	if err != nil || again != 0 {
		t.Fatalf("重跑对账应幂等（0 行），实际 %d, %v", again, err)
	}
}

func TestMuteVisibleInDetailAndAdminList(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)
	if _, err := r.Service().Mute(testCtx(), admin, aliceID, 7); err != nil {
		t.Fatalf("禁言失败: %v", err)
	}

	// 读者设置页靠 Detail 的 mute_until 展示剩余时间。
	detail, err := r.Service().Detail(testCtx(), alice)
	if err != nil {
		t.Fatalf("读详情失败: %v", err)
	}
	if detail.MuteUntil == nil {
		t.Fatal("详情应带 mute_until，设置页才能展示剩余时间")
	}

	// 账号管理页靠 AdminList 的 mute_until 展示"禁言至某时"并给解除按钮。
	items, _, err := r.Service().AdminList(testCtx(), admin, domain.Query{})
	if err != nil {
		t.Fatalf("读管理端列表失败: %v", err)
	}
	var found bool
	for _, item := range items {
		if item.ID == aliceID {
			found = true
			if item.MuteUntil == nil {
				t.Fatal("管理端列表应带 mute_until")
			}
		}
	}
	if !found {
		t.Fatal("管理端列表里找不到被禁言的账号")
	}
}

// 编译期确认用例签名（防"改了签名调用点没跟上"）：这两条在别处也会被用到。
var (
	_ = (*app.Service).Mute
	_ = (*app.Service).Unmute
	_ = (*app.Service).EnsureCanPublish
)

// 管理端列表必须带出邮箱、手机号与简介：管理员排查账号时靠它三样。
//
// 之前列表只有用户名与昵称，"这个人到底绑了哪个手机 / 邮箱对不对 / 简介里
// 有没有留联系方式"只能去翻数据库 —— 而手机号与邮箱恰恰是登录主标识，
// 最容易把同一个人的两个账号混起来。
func TestAdminListCarriesContactAndAbstract(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	seed(t, r)

	email := "alice@example.com"
	phone := "13800001234"
	abstract := "前端开发，接私活请私信"
	r.Users().Seed(domain.User{
		ID:       aliceID,
		Username: "alice",
		Nickname: "Alice",
		Email:    &email,
		Phone:    &phone,
		Abstract: abstract,
		Role:     domain.RoleUser,
		Status:   domain.StatusActive,
	})

	items, _, err := r.Service().AdminList(testCtx(), admin, domain.Query{})
	if err != nil {
		t.Fatalf("读管理端列表失败: %v", err)
	}
	var got *struct {
		email, phone, abstract string
	}
	for _, item := range items {
		if item.ID == aliceID {
			got = &struct {
				email, phone, abstract string
			}{item.Email, item.Phone, item.Abstract}
		}
	}
	if got == nil {
		t.Fatal("管理端列表里找不到目标账号")
	}
	if got.email != email {
		t.Fatalf("列表应带邮箱 %q，实际 %q", email, got.email)
	}
	if got.phone != phone {
		t.Fatalf("列表应带手机号 %q，实际 %q", phone, got.phone)
	}
	if got.abstract != abstract {
		t.Fatalf("列表应带简介 %q，实际 %q", abstract, got.abstract)
	}

	// 未绑定的账号：必须是空串而不是 panic / 空指针（Phone/Email 在领域模型里是 *string）。
	unboundID := ctype.NewID(1002)
	r.Users().Seed(domain.User{
		ID:       unboundID,
		Username: "bobby",
		Nickname: "Bob",
		Role:     domain.RoleUser,
		Status:   domain.StatusActive,
	})
	items, _, err = r.Service().AdminList(testCtx(), admin, domain.Query{})
	if err != nil {
		t.Fatalf("读管理端列表失败: %v", err)
	}
	for _, item := range items {
		if item.ID == unboundID {
			if item.Email != "" || item.Phone != "" {
				t.Fatalf("未绑定邮箱/手机号的账号应回空串，实际 email=%q phone=%q", item.Email, item.Phone)
			}
		}
	}
}
