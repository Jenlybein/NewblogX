// Package apptest 提供 article 模块的有状态测试替身。
//
// 为什么不用 gomock：mock 只能断言"调了哪个方法、传了什么参数"，无法表达状态。
// 本项目最需要测的恰恰是状态——"重复创建返回冲突""事务回滚后仓储无副作用"
// "第二次读能读到第一次写"。这些都是有状态 fake 才能表达的。
//
// 所有 fake 都带故障注入（FailNext）与调用计数（Calls），
// 并实现 Snapshot/RestoreState，供 Tx 在回滚时还原。
package apptest

import "sync"

// fault 是按键注入的一次性故障。
//
// 用一次性而不是常驻：测试里"下一次 Save 失败"比"Save 永远失败"更贴近真实场景，
// 也避免忘了清除导致后续断言莫名失败。
type fault struct {
	mu   sync.Mutex
	next map[string]error
}

// FailNext 让下一次 method 调用返回 err。
func (f *fault) FailNext(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next == nil {
		f.next = make(map[string]error)
	}
	f.next[method] = err
}

// ClearFaults 清空所有待触发故障。
func (f *fault) ClearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next = nil
}

// take 取出并清除 method 的待触发故障。
func (f *fault) take(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next == nil {
		return nil
	}
	err, ok := f.next[method]
	if !ok {
		return nil
	}
	delete(f.next, method)
	return err
}

// counter 统计方法调用次数，供"有没有真的走事务""有没有重复查询"这类断言使用。
type counter struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *counter) inc(method string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = make(map[string]int)
	}
	c.n[method]++
}

// Calls 返回某方法的调用次数。
func (c *counter) Calls(method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		return 0
	}
	return c.n[method]
}

// TotalCalls 返回所有方法的调用次数之和。
func (c *counter) TotalCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, v := range c.n {
		total += v
	}
	return total
}

// snapshotter 是需要参与事务回滚的仓储。
type snapshotter interface {
	// Snapshot 返回当前状态的深拷贝。
	Snapshot() any
	// RestoreState 用快照覆盖当前状态。
	RestoreState(any)
}
