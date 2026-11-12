package app_test

import (
	"errors"
	"fmt"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// 手机号登录与注册是同一条路径：查到就登录、查不到就建档。
// 这组用例守两件事——建档出来的账号长什么样，以及"查库失败"绝不能被当成"没注册过"。

// alloc 是自动用户名分配器替身。
type alloc struct{ n int }

func (a *alloc) Next() (string, error) {
	a.n++
	return fmt.Sprintf("user%d", a.n), nil
}

func TestFindOrCreateByPhone_首次登录建档(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	u, err := svc.FindOrCreateByPhone(testCtx(), "13800001234")
	if err != nil {
		t.Fatalf("首次手机号登录应建档而不是报错: %v", err)
	}
	if u.RegisterSource != domain.SourcePhone {
		t.Fatalf("注册来源应是手机号，实际 %d", u.RegisterSource)
	}
	if u.Phone == nil || *u.Phone != "13800001234" {
		t.Fatalf("手机号未落库: %+v", u.Phone)
	}
	if u.Status != domain.StatusActive || u.Role != domain.RoleUser {
		t.Fatalf("新账号应是启用的普通用户，实际 status=%d role=%d", u.Status, u.Role)
	}
	// 昵称是打码号：昵称会出现在评论、关注列表这些公开位置。
	if u.Nickname != domain.MaskPhone("13800001234") {
		t.Fatalf("默认昵称应是打码手机号，实际 %q", u.Nickname)
	}
}

func TestFindOrCreateByPhone_已有账号时取同一条记录(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	phone := "13800001234"
	seeded := r.Users().Seed(domain.User{
		Username: "old", Nickname: "老用户", Phone: &phone,
		Status: domain.StatusActive, Role: domain.RoleUser,
	})
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	u, err := svc.FindOrCreateByPhone(testCtx(), phone)
	if err != nil {
		t.Fatalf("已注册号码登录失败: %v", err)
	}
	if u.ID != seeded.ID {
		t.Fatalf("应返回既有账号 %s，实际 %s", seeded.ID, u.ID)
	}
}

func TestFindOrCreateByPhone_查库出错时不要建档(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	boom := errors.New("db down")
	r.Users().FailReads(boom)
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	_, err := svc.FindOrCreateByPhone(testCtx(), "13800001234")
	if !errors.Is(err, boom) {
		t.Fatalf("查库失败应原样上抛，实际 %v", err)
	}
	// 关键点：不能因为"读失败"就认为号码没注册过再建一个新账号，
	// 否则库抖动一次，用户登录一次就多一个空账号。
	if _, findErr := r.Users().FindByPhone(testCtx(), "13800001234"); findErr == nil {
		t.Fatal("查库失败时不该建档")
	}
}

func TestFindOrCreateByPhone_非法号码不建档(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	_, err := svc.FindOrCreateByPhone(testCtx(), "1380000")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserPhoneInvalid {
		t.Fatalf("非法号码应返回 %s，实际 %v", apperr.CodeUserPhoneInvalid, err)
	}
}

// 绑定手机号是"给已有账号补一个号"，不是"再建一个账号"。
// 号被别人占着时必须报错 —— 若走成"建档"，同一个号会进两个账号，
// 以后用它登录到底进哪个号就没有答案了。
func TestBindPhone_号码被占用时拒绝(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	taken := "13800001111"
	r.Users().Seed(domain.User{Username: "owner", Phone: &taken})
	mine := r.Users().Seed(domain.User{Username: "mine"})
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	err := svc.BindPhone(testCtx(), mine.ID, taken)
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserPhoneTaken {
		t.Fatalf("应返回 %s，实际 %v", apperr.CodeUserPhoneTaken, err)
	}
	got, err := svc.PhoneOf(testCtx(), mine.ID)
	if err != nil {
		t.Fatalf("回查手机号失败: %v", err)
	}
	if got != "" {
		t.Fatalf("绑定失败后不该写库，实际 %s", got)
	}
}

func TestBindPhone_未占用时写入(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	mine := r.Users().Seed(domain.User{Username: "mine"})
	svc := r.ServiceWith(app.WithUsernameAllocator(&alloc{}))

	if err := svc.BindPhone(testCtx(), mine.ID, "13800002222"); err != nil {
		t.Fatalf("绑定应成功: %v", err)
	}
	got, err := svc.PhoneOf(testCtx(), mine.ID)
	if err != nil {
		t.Fatalf("回查手机号失败: %v", err)
	}
	if got != "13800002222" {
		t.Fatalf("手机号未写入，实际 %q", got)
	}
}
