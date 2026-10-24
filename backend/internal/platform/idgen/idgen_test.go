package idgen

import (
	"sync"
	"testing"
)

// reset 清空全局状态，让每个用例从"未初始化"开始。
// 本文件是包内测试（package idgen），所以能直接碰私有变量。
func reset() {
	mu.Lock()
	node = nil
	initializedID = 0
	mu.Unlock()
}

func TestInitRejectsInvalidWorkerID(t *testing.T) {
	t.Cleanup(reset)
	reset()

	for _, id := range []uint32{0, MaxWorkerID + 1, 9999} {
		if err := Init(id); err == nil {
			t.Fatalf("worker id %d 应被拒绝", id)
		}
	}
	if MustReady() {
		t.Fatal("非法机器号不应让生成器就绪")
	}
}

func TestNextRequiresInit(t *testing.T) {
	t.Cleanup(reset)
	reset()

	if _, err := Next(); err == nil {
		t.Fatal("未初始化时 Next 必须报错，不能兜底")
	}
}

func TestInitIdempotentAndRejectsSwitch(t *testing.T) {
	t.Cleanup(reset)
	reset()

	if err := Init(7); err != nil {
		t.Fatalf("首次初始化失败: %v", err)
	}
	if err := Init(7); err != nil {
		t.Fatalf("同机器号重复初始化应幂等，实际: %v", err)
	}
	if err := Init(8); err == nil {
		t.Fatal("切换机器号必须报错（会生成重复 ID）")
	}
	if !MustReady() {
		t.Fatal("初始化后应就绪")
	}
}

func TestNextIsUniqueUnderConcurrency(t *testing.T) {
	t.Cleanup(reset)
	reset()

	if err := Init(1); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	const goroutines = 8
	const perGoroutine = 500

	var mu sync.Mutex
	seen := make(map[uint64]struct{}, goroutines*perGoroutine)
	var wg sync.WaitGroup
	var firstErr error

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id, err := Next()
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				if _, dup := seen[id.Uint64()]; dup {
					mu.Unlock()
					t.Errorf("生成重复 ID: %d", id.Uint64())
					return
				}
				seen[id.Uint64()] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("并发生成出错: %v", firstErr)
	}
	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("ID 数量不对: got %d want %d", len(seen), goroutines*perGoroutine)
	}
}

func TestNextIsMonotonic(t *testing.T) {
	t.Cleanup(reset)
	reset()

	if err := Init(3); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	prev, err := Next()
	if err != nil {
		t.Fatalf("Next 失败: %v", err)
	}
	for i := 0; i < 100; i++ {
		cur, err := Next()
		if err != nil {
			t.Fatalf("Next 失败: %v", err)
		}
		if cur.Uint64() <= prev.Uint64() {
			t.Fatalf("雪花 ID 应严格递增: prev=%d cur=%d", prev.Uint64(), cur.Uint64())
		}
		prev = cur
	}
}
