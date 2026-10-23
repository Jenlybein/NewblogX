package redis_user_test

import (
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_user"
	"myblogx/test/testutil"
	"testing"
	"time"
)

func TestTryMarkUserHomeViewed(t *testing.T) {
	mr := testutil.SetupMiniRedis(t)
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	now := time.Date(2026, 3, 30, 21, 30, 0, 0, time.Local)

	marked, err := redis_user.TryMarkUserHomeViewed(deps, 2, 1, now)
	if err != nil {
		t.Fatalf("首次写入主页访问判重失败: %v", err)
	}
	if !marked {
		t.Fatal("首次访问应判定为当天首次")
	}

	marked, err = redis_user.TryMarkUserHomeViewed(deps, 2, 1, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("重复写入主页访问判重失败: %v", err)
	}
	if marked {
		t.Fatal("同一访客当天重复访问不应再次记数")
	}

	marked, err = redis_user.TryMarkUserHomeViewed(deps, 2, 3, now)
	if err != nil {
		t.Fatalf("不同访客写入主页访问判重失败: %v", err)
	}
	if !marked {
		t.Fatal("不同访客当天首次访问应记数")
	}

	mr.FastForward(3 * time.Hour)
	marked, err = redis_user.TryMarkUserHomeViewed(deps, 2, 1, now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("跨天写入主页访问判重失败: %v", err)
	}
	if !marked {
		t.Fatal("跨天后应允许再次记数")
	}
}

func TestRollbackUserHomeViewed(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	now := time.Date(2026, 3, 30, 21, 0, 0, 0, time.Local)

	marked, err := redis_user.TryMarkUserHomeViewed(deps, 2, 1, now)
	if err != nil || !marked {
		t.Fatalf("预写主页访问判重失败: marked=%v err=%v", marked, err)
	}
	if err = redis_user.RollbackUserHomeViewed(deps, 2, 1, now); err != nil {
		t.Fatalf("回滚主页访问判重失败: %v", err)
	}

	marked, err = redis_user.TryMarkUserHomeViewed(deps, 2, 1, now)
	if err != nil {
		t.Fatalf("回滚后再次写入主页访问判重失败: %v", err)
	}
	if !marked {
		t.Fatal("回滚后应允许再次记数")
	}
}
