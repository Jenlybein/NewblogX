// Package testkit 提供无全局状态的测试基建。
//
// 与旧的 test/testutil 的根本差别：
//   - 不依赖包级变量与 InitGlobals()，每个测试各自持有自己的依赖；
//   - 所有助手都能在 t.Parallel() 下安全使用（资源按测试隔离，或用 t.Cleanup 释放）；
//   - 时间、ID 可注入，避免断言依赖真实时钟。
//
// 约定：助手只做“搭环境”，不含任何业务断言。
package testkit

import (
	"sync"
	"time"
)

// Clock 抽象“现在”。业务逻辑依赖它而不是 time.Now，时间相关分支才可确定性测试。
type Clock interface {
	Now() time.Time
}

// RealClock 使用系统时间，供生产装配使用。
type RealClock struct{}

// Now 返回系统当前时间。
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock 是可控时钟，并发安全。
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock 创建一个停在 at 的假时钟。
func NewFakeClock(at time.Time) *FakeClock {
	return &FakeClock{now: at}
}

// Now 返回当前（被设定的）时间。
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance 前进 d 并返回新的当前时间。d 为负表示回拨。
func (c *FakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

// Set 把时钟拨到 t。
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
