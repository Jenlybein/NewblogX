package ai

import (
	"context"

	"myblogx/internal/platform/llm"
)

// CallResult 是一次 AI 调用的终态快照，交给装配层落日志。
type CallResult struct {
	Meta   CallMeta
	Usage  llm.CallUsage
	Stream bool
	// Err 为 nil 表示成功。失败时 Usage 里通常没有用量（上游在错误响应里不回 token），
	// 但 Attempts 仍然是真实的请求次数。
	Err error
}

// CallReporter 是落库回调。
type CallReporter func(ctx context.Context, res CallResult)

// callReporter 是进程级单例，与 llm.SharedFactory 同类。
//
// 由装配层（app.WireAI）安装一次；为 nil 时全部空转 —— 单测与命令行工具
// 不需要关心日志管道是否就位。
var callReporter CallReporter

// SetCallReporter 安装落库回调。重复调用等于覆盖，语义上无害，
// 但只应该在装配层调一次。
func SetCallReporter(fn CallReporter) { callReporter = fn }

// reportCall 是 ai.Chat / ai.ChatStream 的唯一上报点。
//
// 注意两点：
//  1. 一切失败都要上报（不只是"重试耗尽"），否则后台看不出失败率，
//     而失败率才是"模型到底可用不可用"的唯一证据；
//  2. 这里只做投递，不做 IO —— 日志发射器入队即返回（有界队列满了会丢并计数），
//     所以它不会给请求路径增加延迟，也不会因为写盘慢而拖住 SSE。
func reportCall(ctx context.Context, usage llm.CallUsage, err error, stream bool) {
	if callReporter == nil {
		return
	}
	callReporter(ctx, CallResult{
		Meta:   callMetaFrom(ctx),
		Usage:  usage,
		Stream: stream,
		Err:    err,
	})
}
