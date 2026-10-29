package ai

import (
	"context"

	"myblogx/internal/kernel/ctype"
)

// CallMeta 是一次 AI 调用的语义元信息：谁在用、用在哪。
//
// 为什么放 ctx 而不是加函数参数：调用链是
// handler → app.Service → 6 个业务子包 → ai.Chat，
// 加参数要改 10 个调用点 + 6 个子包签名，而这些信息对业务逻辑本身毫无用途，
// 纯粹是"给日志看的"。ctx 本来就已经全链路透传（客户端断开要能取消上游请求）。
type CallMeta struct {
	// Scene 是功能场景，见下面 6 个常量。空值表示"没标注"，后台会显示为 -。
	Scene string
	// UserID 是触发这次调用的用户；零值表示匿名或异步任务（如 outbox 补评分）。
	//
	// 类型必须是 ctype.ID 而不能是 uint64：雪花 ID 超出 JS 安全整数，
	// 裸整型会让契约把它印成 integer，前端拿到的 ID 会悄悄丢精度（R12 会拦）。
	UserID    ctype.ID
	RequestID string
	TraceID   string
}

// 场景标识。
//
// 集中一处定义而不是在 6 个子包里各写字面量：后台的场景筛选下拉、首页图例、
// 日志上报三处都读它，分散写就会出现 "search_answer" 与 "searchAnswer"
// 这种永远匹配不上的脏维度，而且不会报错。
const (
	SceneScoring       = "scoring"
	SceneMetainfo      = "metainfo"
	SceneOverwrite     = "overwrite"
	SceneDiagnose      = "diagnose"
	SceneSearchRewrite = "search_rewrite"
	SceneSearchAnswer  = "search_answer"
)

// callMetaKey 是 ctx 里存取 CallMeta 的私有键类型（避免和其他包的键撞车）。
type callMetaKey struct{}

// WithCallMeta 在 ctx 上合并一次补丁，非零字段覆盖。
//
// 合并而不是覆盖整份：handler 只知道"谁在调"（UserID/RequestID），
// service 只知道"调的是哪个功能"（Scene），两处先后写入不该互相清空。
//
// 返回新 ctx 而不改原对象：ctx 可能被多个 goroutine 共享（比如 SSE 写协程），
// 原地改内部指针就是数据竞争。
func WithCallMeta(ctx context.Context, patch CallMeta) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := callMetaFrom(ctx)
	if patch.Scene != "" {
		current.Scene = patch.Scene
	}
	if !patch.UserID.IsZero() {
		current.UserID = patch.UserID
	}
	if patch.RequestID != "" {
		current.RequestID = patch.RequestID
	}
	if patch.TraceID != "" {
		current.TraceID = patch.TraceID
	}
	return context.WithValue(ctx, callMetaKey{}, current)
}

// callMetaFrom 取出当前的元信息；没有就返回零值。
func callMetaFrom(ctx context.Context) CallMeta {
	if ctx == nil {
		return CallMeta{}
	}
	if value, ok := ctx.Value(callMetaKey{}).(CallMeta); ok {
		return value
	}
	return CallMeta{}
}
