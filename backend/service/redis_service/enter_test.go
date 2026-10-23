package redis_service_test

import (
	"context"
	"myblogx/service/redis_service"
	"myblogx/test/testutil"
	"testing"
	"time"
)

func TestLockArticleSync(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	ctx := context.Background()
	key := "lock:article:sync"
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}

	unlock, err := redis_service.LockArticleSync(deps, ctx, key, 5*time.Second)
	if err != nil {
		t.Fatalf("首次加锁失败: %v", err)
	}
	if unlock == nil {
		t.Fatal("首次加锁应成功并返回解锁函数")
	}

	unlock2, err := redis_service.LockArticleSync(deps, ctx, key, 5*time.Second)
	if err != nil {
		t.Fatalf("重复加锁不应报错: %v", err)
	}
	if unlock2 != nil {
		t.Fatal("锁已被占用时应返回 nil 解锁函数")
	}

	unlock()

	unlock3, err := redis_service.LockArticleSync(deps, ctx, key, 5*time.Second)
	if err != nil {
		t.Fatalf("释放后再次加锁失败: %v", err)
	}
	if unlock3 == nil {
		t.Fatal("释放后应可重新加锁")
	}
}
