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

	// Get 必须传入**同形状**的 Dest：缓存键包含目标形状，
	// 形状不同的查询不该命中同一份结果（否则字段会被静默归零）。
	out := &caches.Query[any]{Dest: map[string]any{}}
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

// TestRedisCacherSeparatesQueryShapes 钉住 #132 的核心：
// 同一个 SQL、不同目标形状的查询，不能命中同一份缓存结果。
//
// 插件的缓存标识只由「SQL + 参数」组成，不含目标结构体类型；若不在这里
// 补形状，窄行查询存进去的 JSON 会被整行查询命中，缺失字段静默归零。
func TestRedisCacherSeparatesQueryShapes(t *testing.T) {
	mr := testutil.SetupMiniRedis(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	c := &redisCacher{rdb: rdb}
	ctx := context.Background()
	key := caches.IdentifierPrefix + "shape:test"

	// 窄形状存进去。
	if err := c.Store(ctx, key, &caches.Query[any]{
		Dest:         map[string]any{"name": "alice"},
		RowsAffected: 1,
	}); err != nil {
		t.Fatalf("Store 失败: %v", err)
	}

	// 同形状能取到。
	same, err := c.Get(ctx, key, &caches.Query[any]{Dest: map[string]any{}})
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if same == nil {
		t.Fatal("同形状应命中缓存")
	}

	// 不同形状**不应**命中：整行查询若命中窄 JSON，缺的字段会静默归零。
	other, err := c.Get(ctx, key, &caches.Query[any]{Dest: struct{ Name string }{}})
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if other != nil {
		t.Fatalf("不同形状不应命中同一份缓存，实际 got=%#v", other.Dest)
	}
}
