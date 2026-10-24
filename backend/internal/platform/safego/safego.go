// Package safego 提供带 panic 保护的 goroutine 启动器。
//
// 项目内所有后台协程都必须经由本包启动。裸 go 语句一旦 panic，会直接终止
// 整个进程——gin 的 Recovery 只覆盖 handler 自己的 goroutine，管不到后台任务。
// archcheck 的 R7 规则会拦裸 go，且仅对本包豁免：这里是唯一的出口。
package safego

import "runtime/debug"

// PanicHandler 接收 panic 值与当时的调用栈。
//
// 由调用方决定如何记录（通常交给结构化日志），safego 自身不依赖任何日志实现，
// 以免日志管道反过来依赖 safego 造成环。
type PanicHandler func(recovered any, stack []byte)

// Go 启动 fn，并在 fn panic 时恢复，避免进程退出。
//
// handler 为 nil 时静默恢复（只应在测试或确定无副作用时使用）。
// Go 不等待 fn 结束；需要同步请由调用方自行用 sync.WaitGroup 或其他信号。
func Go(handler PanicHandler, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if handler != nil {
					handler(r, debug.Stack())
				}
			}
		}()
		fn()
	}()
}
