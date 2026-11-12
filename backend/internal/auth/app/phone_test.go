package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/app"
	"myblogx/internal/auth/apptest"
	"myblogx/internal/kernel/ctype"
)

// 这组用例守两条安全线：
//  1. 手机号登录只回答"这个号对应谁"，号码形状不对就不该去碰账号目录；
//  2. 首次设密码必须同时满足"已登录""号码与绑定一致""短信复验通过"，
//     少任何一条都等于给被盗会话留一条免旧密码改密的后门。

// dirStub 只实现手机号链路用到的方法。嵌入 app.Directory 是刻意的：
// 一旦用例走到别的目录方法就会 nil panic，逼着补测试而不是静默通过。
type dirStub struct {
	app.Directory
	bound    string
	account  app.Account
	found    int
	password string
}

func (d *dirStub) FindOrCreateByPhone(_ context.Context, _ string) (app.Account, error) {
	d.found++
	return d.account, nil
}

func (d *dirStub) PhoneOf(_ context.Context, _ uint64) (string, error) { return d.bound, nil }

func (d *dirStub) SetPassword(_ context.Context, _ uint64, newPassword string) error {
	d.password = newPassword
	return nil
}

// smsStub 是短信核验替身。
type smsStub struct {
	ok   bool
	err  error
	sent int
}

func (s *smsStub) Send(_ context.Context, _ int8, _, _ string) error {
	s.sent++
	return s.err
}

func (s *smsStub) Check(_ context.Context, _, _ string) (bool, error) { return s.ok, s.err }

func phoneService(t *testing.T, d *dirStub, s *smsStub) *app.Service {
	t.Helper()
	repos := apptest.NewRepos(baseTime)
	return repos.ServiceWith(app.WithDirectory(d), app.WithSmsCodes(s))
}

func TestLoginWithPhone_空号码不碰账号目录(t *testing.T) {
	t.Parallel()
	d := &dirStub{}
	svc := phoneService(t, d, &smsStub{})

	_, err := svc.LoginWithPhone(context.Background(), "   ", meta())
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserPhoneInvalid {
		t.Fatalf("空号码应返回 %s，实际 %v", apperr.CodeUserPhoneInvalid, err)
	}
	if d.found != 0 {
		t.Fatal("号码形状不对时不该查账号目录")
	}
}

func TestLoginWithPhone_取到账号后建立会话(t *testing.T) {
	t.Parallel()
	d := &dirStub{account: app.Account{
		ID: ctype.NewID(7), Username: "u7", Role: 2, TokenVersion: 1,
	}}
	svc := phoneService(t, d, &smsStub{})

	res, err := svc.LoginWithPhone(context.Background(), "13800001234", meta())
	if err != nil {
		t.Fatalf("手机号登录失败: %v", err)
	}
	if d.found != 1 {
		t.Fatalf("应只查一次账号目录，实际 %d", d.found)
	}
	if res == nil || res.Account.Username != "u7" {
		t.Fatalf("返回结果不对: %+v", res)
	}
}

func TestSetPassword_号码与绑定不一致时拒绝(t *testing.T) {
	t.Parallel()
	d := &dirStub{bound: "13800001111"}
	sms := &smsStub{ok: true}
	svc := phoneService(t, d, sms)

	err := svc.SetPassword(context.Background(), 7, "13800002222", "123456", "secret123")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserPhoneInvalid {
		t.Fatalf("换号设密码应返回 %s，实际 %v", apperr.CodeUserPhoneInvalid, err)
	}
	if d.password != "" {
		t.Fatal("号码不一致时不该写入密码")
	}
}

func TestSetPassword_未绑定手机号时拒绝(t *testing.T) {
	t.Parallel()
	d := &dirStub{bound: ""}
	svc := phoneService(t, d, &smsStub{ok: true})

	err := svc.SetPassword(context.Background(), 7, "", "", "secret123")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserPhoneInvalid {
		t.Fatalf("未绑定手机号应返回 %s，实际 %v", apperr.CodeUserPhoneInvalid, err)
	}
	if d.password != "" {
		t.Fatal("未绑定手机号时不该写入密码")
	}
}

func TestSetPassword_短信复验不通过时拒绝(t *testing.T) {
	t.Parallel()
	d := &dirStub{bound: "13800001111"}
	svc := phoneService(t, d, &smsStub{ok: false})

	err := svc.SetPassword(context.Background(), 7, "13800001111", "000000", "secret123")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserCaptchaInvalid {
		t.Fatalf("验证码错应返回 %s，实际 %v", apperr.CodeUserCaptchaInvalid, err)
	}
	if d.password != "" {
		t.Fatal("复验没通过时不该写入密码")
	}
}

func TestSetPassword_复验通过后写入(t *testing.T) {
	t.Parallel()
	d := &dirStub{bound: "13800001111"}
	svc := phoneService(t, d, &smsStub{ok: true})

	if err := svc.SetPassword(context.Background(), 7, "13800001111", "123456", "secret123"); err != nil {
		t.Fatalf("设密码失败: %v", err)
	}
	if d.password != "secret123" {
		t.Fatalf("密码未写入，实际 %q", d.password)
	}
}

func TestSetPassword_未登录直接拒绝(t *testing.T) {
	t.Parallel()
	d := &dirStub{bound: "13800001111"}
	svc := phoneService(t, d, &smsStub{ok: true})

	err := svc.SetPassword(context.Background(), 0, "13800001111", "123456", "secret123")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeAuthRequired {
		t.Fatalf("未登录应返回 %s，实际 %v", apperr.CodeAuthRequired, err)
	}
	if d.password != "" {
		t.Fatal("未登录时不该写入密码")
	}
}

func TestSendSmsCode_透传到底层并在失败时上抛(t *testing.T) {
	t.Parallel()
	d := &dirStub{}
	boom := errors.New("sms down")
	sms := &smsStub{err: boom}
	svc := phoneService(t, d, sms)

	if err := svc.SendSmsCode(context.Background(), 1, "13800001234", "1.2.3.4"); !errors.Is(err, boom) {
		t.Fatalf("发送失败应上抛, 实际 %v", err)
	}
	if sms.sent != 1 {
		t.Fatalf("应调用一次发送，实际 %d", sms.sent)
	}
}
