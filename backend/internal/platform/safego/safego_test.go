package safego_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"myblogx/internal/platform/safego"
)

// 正常路径：fn 会执行完。
func TestGoRunsFn(t *testing.T) {
	t.Parallel()

	var ran atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	safego.Go(func(any, []byte) {
		t.Error("正常执行不应触发 PanicHandler")
	}, func() {
		ran.Store(true)
		wg.Done()
	})

	wg.Wait()
	if !ran.Load() {
		t.Fatal("fn 未被执行")
	}
}

// panic 路径：必须被恢复，且 handler 拿到值与调用栈。
func TestGoRecoversPanic(t *testing.T) {
	t.Parallel()

	var (
		wg        sync.WaitGroup
		recovered any
		stackLen  int
	)
	wg.Add(1)

	safego.Go(func(r any, stack []byte) {
		recovered = r
		stackLen = len(stack)
		wg.Done()
	}, func() {
		panic("boom")
	})

	wg.Wait()

	if recovered == nil {
		t.Fatal("PanicHandler 未收到 panic 值")
	}
	if s, ok := recovered.(string); !ok || s != "boom" {
		t.Fatalf("panic 值不符：%v", recovered)
	}
	if stackLen == 0 {
		t.Fatal("调用栈为空，无法定位 panic 现场")
	}
}

// handler 为 nil 时必须静默恢复，不能把进程带走。
func TestGoNilHandlerStillRecovers(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	safego.Go(nil, func() { panic("silent") })
	safego.Go(nil, func() { close(done) })

	<-done // 能走到这里说明 panic 已被吞掉，进程未退出
}
