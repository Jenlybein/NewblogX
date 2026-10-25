package rediscounter_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/platform/rediscounter"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// setup 起一个**独立的** miniredis 实例。
//
// 不用 testutil.SetupMiniRedis：它把 client 放在全局变量里，每个测试覆盖同一个全局，
// 配合 t.Parallel() 会互相清空连接（现象是随机 "connection refused"
// 或读到别的测试写的数据）。
func setup(t *testing.T) (*rediscounter.Flusher, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})
	// logger 传 nil：本文件故意制造失败路径，不需要把告警打进测试输出。
	return rediscounter.New(client, nil), client
}

func newTask(apply func(id uint64, delta int) error) rediscounter.Task {
	return rediscounter.Task{
		Name:       "测试任务",
		Metric:     "计数",
		ActiveKey:  "test:active",
		SyncKey:    "test:syncing",
		IDName:     "item_id",
		ApplyDelta: apply,
	}
}

// TestFlushAppliesDeltas 验证正常路径：读到增量、逐条落库、清掉 sync key。
func TestFlushAppliesDeltas(t *testing.T) {
	t.Parallel()
	f, client := setup(t)
	ctx := context.Background()

	client.HSet(ctx, "test:active", "1", "5")
	client.HSet(ctx, "test:active", "2", "3")

	got := map[uint64]int{}
	if _, err := f.Flush(ctx, newTask(func(id uint64, delta int) error {
		got[id] = delta
		return nil
	})); err != nil {
		t.Fatalf("Flush 返回错误: %v", err)
	}

	if got[1] != 5 || got[2] != 3 {
		t.Fatalf("落库增量不符: %+v", got)
	}
	if n := client.Exists(ctx, "test:syncing").Val(); n != 0 {
		t.Fatalf("sync key 应被清掉，实际还存在")
	}
}

// TestFlushNoActiveKey 无增量时不做任何事，也不应报错。
func TestFlushNoActiveKey(t *testing.T) {
	t.Parallel()
	f, _ := setup(t)
	ctx := context.Background()

	called := false
	n, err := f.Flush(ctx, newTask(func(uint64, int) error { called = true; return nil }))
	if err != nil {
		t.Fatalf("Flush 返回错误: %v", err)
	}
	if called {
		t.Fatal("没有增量时不应调用 ApplyDelta")
	}
	if n != 0 {
		t.Fatalf("应返回 0 条，实际 %d", n)
	}
}

// TestFlushRequeuesOnApplyError 是本包最关键的语义：
// 写库失败必须把增量加回 active key，否则这批数据永久丢失。
func TestFlushRequeuesOnApplyError(t *testing.T) {
	t.Parallel()
	f, client := setup(t)
	ctx := context.Background()

	client.HSet(ctx, "test:active", "7", "4")

	sentinel := errors.New("db down")
	if _, err := f.Flush(ctx, newTask(func(uint64, int) error { return sentinel })); err != nil {
		t.Fatalf("单条落库失败不应让 Flush 整体报错: %v", err)
	}

	left := client.HGet(ctx, "test:active", "7").Val()
	if left != "4" {
		t.Fatalf("增量应被回补到 active key，实际=%q", left)
	}
}

// TestFlushSkipsInvalidEntries 非法 id / 增量只跳过该条，其余照常落库。
func TestFlushSkipsInvalidEntries(t *testing.T) {
	t.Parallel()
	f, client := setup(t)
	ctx := context.Background()

	client.HSet(ctx, "test:active", "not-a-number", "3")
	client.HSet(ctx, "test:active", "9", "not-a-number")
	client.HSet(ctx, "test:active", "10", "0")
	client.HSet(ctx, "test:active", "11", "2")

	got := map[uint64]int{}
	if _, err := f.Flush(ctx, newTask(func(id uint64, delta int) error {
		got[id] = delta
		return nil
	})); err != nil {
		t.Fatalf("Flush 返回错误: %v", err)
	}

	if len(got) != 1 || got[11] != 2 {
		t.Fatalf("应只落库 id=11 增量=2，实际 %+v", got)
	}
}

// TestFlushKeepsConcurrentWrites 验证双缓冲的意义：
// 刷盘期间业务写到 active key 的新增量必须保留，不能被这次刷盘吞掉。
func TestFlushKeepsConcurrentWrites(t *testing.T) {
	t.Parallel()
	f, client := setup(t)
	ctx := context.Background()

	client.HSet(ctx, "test:active", "1", "5")

	task := newTask(func(id uint64, delta int) error {
		// 模拟"刷盘进行中，业务又点了一次赞"：此时 active key 已经被 RENAME 走，
		// 这条写入落在新的 active key 上。
		client.HIncrBy(ctx, "test:active", "1", 1)
		return nil
	})

	if _, err := f.Flush(ctx, task); err != nil {
		t.Fatalf("Flush 返回错误: %v", err)
	}

	left := client.HGet(ctx, "test:active", "1").Val()
	if left != "1" {
		t.Fatalf("刷盘期间的新增量应保留 1，实际=%q", left)
	}
}

// TestFlushResumesFromSyncKey 上一轮崩溃留下的 sync key 会被继续处理。
//
// 这是**恢复语义**而不是跳过：上一轮在 RENAME 之后、写完库之前崩掉，
// 残留的增量只存在于 sync key 里。跳过它就等于永久丢数据。
//
// 正确性依赖外层分布式锁（同一时刻只有一个实例在跑）。没有锁时，
// 两个实例会同时处理同一份残留，把同一笔增量加两遍 —— 所以本包不自己加锁，
// 但不加锁就必须由调用方保证互斥。
func TestFlushResumesFromSyncKey(t *testing.T) {
	t.Parallel()
	f, client := setup(t)
	ctx := context.Background()

	// 只剩 sync key：active 已被上一轮 RENAME 走，但没写完库。
	client.HSet(ctx, "test:syncing", "3", "7")

	got := map[uint64]int{}
	n, err := f.Flush(ctx, newTask(func(id uint64, delta int) error {
		got[id] = delta
		return nil
	}))
	if err != nil {
		t.Fatalf("Flush 返回错误: %v", err)
	}
	if n != 1 || got[3] != 7 {
		t.Fatalf("应恢复处理残留增量 id=3 delta=7，实际 n=%d got=%+v", n, got)
	}
	if v := client.Exists(ctx, "test:syncing").Val(); v != 0 {
		t.Fatal("处理完应清掉 sync key")
	}
}

// TestFlushNilClient 未配置 Redis 时静默返回（表里没有增量可言）。
func TestFlushNilClient(t *testing.T) {
	t.Parallel()
	f := rediscounter.New(nil, nil)
	n, err := f.Flush(context.Background(), newTask(func(uint64, int) error { return nil }))
	if err != nil || n != 0 {
		t.Fatalf("无 Redis 时应返回 (0,nil)，实际 (%d,%v)", n, err)
	}
}
