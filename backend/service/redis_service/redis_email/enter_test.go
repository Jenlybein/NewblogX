package redis_email_test

import (
	"myblogx/service/redis_service"
	redis_email "myblogx/service/redis_service/redis_email"
	"myblogx/test/testutil"
	"testing"
	"time"
)

func testRedisDeps() redis_service.Deps {
	return redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
}

func TestEmailVerifyStoreSuccess(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	if err := redis_email.Store(deps, "id1", "a@example.com", "123456", 1, 3); err != nil {
		t.Fatalf("存储验证码失败: %v", err)
	}

	email, ok, err := redis_email.Verify(deps, "id1", "123456")
	if err != nil {
		t.Fatalf("校验验证码异常: %v", err)
	}
	if !ok {
		t.Fatal("验证码应校验成功")
	}
	if email != "a@example.com" {
		t.Fatalf("邮箱不一致: %s", email)
	}

	if _, ok, err = redis_email.Verify(deps, "id1", "123456"); err != nil || ok {
		t.Fatal("成功校验后应被删除")
	}
}

func TestEmailVerifyStoreFailCount(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	if err := redis_email.Store(deps, "id2", "b@example.com", "654321", 1, 2); err != nil {
		t.Fatalf("存储验证码失败: %v", err)
	}

	if _, ok, err := redis_email.Verify(deps, "id2", "000000"); err != nil || ok {
		t.Fatal("错误验证码不应通过")
	}
	if _, ok, err := redis_email.Verify(deps, "id2", "000000"); err != nil || ok {
		t.Fatal("错误验证码不应通过")
	}
	if _, ok, err := redis_email.Verify(deps, "id2", "654321"); err != nil || ok {
		t.Fatal("超出失败次数后应删除")
	}
}

func TestEmailVerifyStoreTimeoutAndDelete(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	if err := redis_email.Store(deps, "id3", "c@example.com", "111111", 0, 3); err != nil {
		t.Fatalf("存储验证码失败: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, ok, err := redis_email.Verify(deps, "id3", "111111"); err != nil || ok {
		t.Fatal("超时后不应通过")
	}

	if err := redis_email.Store(deps, "id4", "d@example.com", "222222", 1, 3); err != nil {
		t.Fatalf("存储验证码失败: %v", err)
	}
	if err := redis_email.Delete(deps, "id4"); err != nil {
		t.Fatalf("删除验证码失败: %v", err)
	}
	if _, ok, err := redis_email.Verify(deps, "id4", "222222"); err != nil || ok {
		t.Fatal("Delete 后不应通过")
	}
}
