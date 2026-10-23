package core

import (
	"context"
	"myblogx/test/testutil"
	"testing"

	"github.com/go-gorm/caches/v4"
	"github.com/go-redis/redis/v8"
)

func TestRedisCacherStoreGetAndInvalidate(t *testing.T) {
	mr := testutil.SetupMiniRedis(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	c := &redisCacher{rdb: rdb}
	ctx := context.Background()

	key := caches.IdentifierPrefix + "unit:test:key"
	in := &caches.Query[any]{
		Dest:         map[string]any{"name": "alice", "age": float64(18)},
		RowsAffected: 1,
	}
	if err := c.Store(ctx, key, in); err != nil {
		t.Fatalf("Store 失败: %v", err)
	}

	out := &caches.Query[any]{}
	got, err := c.Get(ctx, key, out)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got == nil {
		t.Fatal("Get 应返回非空查询结果")
	}
	if got.RowsAffected != 1 {
		t.Fatalf("RowsAffected 错误: %d", got.RowsAffected)
	}

	dest, ok := got.Dest.(map[string]any)
	if !ok || dest["name"] != "alice" {
		t.Fatalf("Dest 数据错误: %#v", got.Dest)
	}

	missing, err := c.Get(ctx, caches.IdentifierPrefix+"missing", &caches.Query[any]{})
	if err != nil {
		t.Fatalf("读取不存在 key 不应报错: %v", err)
	}
	if missing != nil {
		t.Fatalf("不存在 key 时应返回 nil, got=%#v", missing)
	}

	if err := c.Invalidate(ctx); err != nil {
		t.Fatalf("Invalidate 失败: %v", err)
	}
	left := mr.DB(0).Keys()
	if len(left) != 0 {
		t.Fatalf("Invalidate 后应清空缓存, 剩余=%v", left)
	}
}
