package writer

import (
	"fmt"
	"strings"

	"myblogx/internal/kernel/ctype"
	platformlog "myblogx/internal/platform/log"
)

// 本文件是 AI 模型调用的用量日志（ai_call_logs）。
//
// 存在的理由：token 消耗是唯一"用了多少"的账单口径，而它既不在业务库里
// 也不在访问日志里 —— 每次调用的 PromptTokens / CompletionTokens 只在上游
// 响应里出现一次，不在这里落下来就永久丢失。
//
// 刻意**不记** prompt、回复正文与任何请求头：日志管道有 32KB 单条上限
// （emit.go 的 MaxEventBytes），超限是整条丢弃而不是截断；而且正文属于
// 用户内容，日志目录的访问权限比业务库宽松得多。

// AICallEvent 是落 ai_call_logs 的行。
//
// 字段与 deploy/docker/clickhouse/init/01-logs.sql 的列一一对应，
// 但真正的对齐保障是 pipeline_integration_test.go 的 DESCRIBE 逐列比对。
type AICallEvent struct {
	baseEvent
	Scene            string `json:"scene"`
	Vendor           string `json:"vendor"`
	Model            string `json:"model"`
	Stream           uint8  `json:"stream"`
	Success          uint8  `json:"success"`
	Attempts         uint8  `json:"attempts"`
	PromptTokens     uint32 `json:"prompt_tokens"`
	CompletionTokens uint32 `json:"completion_tokens"`
	TotalTokens      uint32 `json:"total_tokens"`
	CachedTokens     uint32 `json:"cached_tokens"`
	ReasoningTokens  uint32 `json:"reasoning_tokens"`
	UsageAvailable   uint8  `json:"usage_available"`
	LatencyMS        uint32 `json:"latency_ms"`
}

// AICallLogInput 是业务侧的入参。
//
// R9 约束：这里的字段必须是标量 —— 禁 any、禁同包自定义类型、
// Extra 只能是 map[string]string。日志管道要能在不反射大对象的前提下编码。
type AICallLogInput struct {
	Scene   string
	Vendor  string
	Model   string
	Stream  bool
	Success bool
	// Attempts 是实际发往上游的次数（含重试）。
	// 失败尝试拿不到用量，所以 attempts=3 的行真实成本可能是一次成功用量的 1~3 倍。
	Attempts int
	// UsageAvailable=false 表示上游根本没回用量（不是"用了 0 token"）。
	UsageAvailable   bool
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	ReasoningTokens  int
	LatencyMS        int64

	UserID       ctype.ID
	RequestID    string
	TraceID      string
	ErrorCode    string
	ErrorMessage string
	Extra        map[string]string
}

// EmitAICallLog 落一条 AI 调用记录。
//
// 成功 info、失败 warn：失败不是"系统故障"（用户输错、上游 429 都算正常波动），
// 用 error 会把 error 级日志的检索价值冲淡。
func EmitAICallLog(deps Options, input AICallLogInput) {
	level := "info"
	eventName := "ai_call_success"
	if !input.Success {
		level = "warn"
		eventName = "ai_call_failed"
	}

	scene := strings.TrimSpace(input.Scene)
	vendor := strings.TrimSpace(input.Vendor)
	// model 来自管理员填写的配置，长度不受控，截一下防止把整条日志撑爆。
	model, _ := truncateByBytes(strings.TrimSpace(input.Model), 128)

	base := newBaseEvent(deps, "ai_call", level, buildAICallMessage(scene, vendor, model))
	base.RequestID = input.RequestID
	base.TraceID = defaultIfEmptyString(input.TraceID, input.RequestID)
	base.EventName = eventName
	base.ErrorCode = input.ErrorCode
	// 上游报错常把整段 HTTP body 塞在 message 里，截到 512B 再入库。
	base.ErrorMessage, _ = truncateByBytes(strings.TrimSpace(input.ErrorMessage), 512)
	base.UserID = input.UserID.Uint64()

	extra := make(map[string]any, len(input.Extra)+1)
	for key, value := range input.Extra {
		extra[key] = value
	}
	// 让"上游没回用量"在数据里可见：否则 0 token 会被当成"这个模型不要钱"。
	if !input.UsageAvailable {
		extra["usage_source"] = "missing"
	}
	base.ExtraJSON = marshalExtra(extra)

	event := AICallEvent{
		baseEvent:        base,
		Scene:            scene,
		Vendor:           vendor,
		Model:            model,
		Stream:           boolToUint8(input.Stream),
		Success:          boolToUint8(input.Success),
		Attempts:         clampToUint8(input.Attempts),
		PromptTokens:     clampToUint32(input.PromptTokens),
		CompletionTokens: clampToUint32(input.CompletionTokens),
		TotalTokens:      clampToUint32(input.TotalTokens),
		CachedTokens:     clampToUint32(input.CachedTokens),
		ReasoningTokens:  clampToUint32(input.ReasoningTokens),
		UsageAvailable:   boolToUint8(input.UsageAvailable),
		LatencyMS:        clampToUint32(int(input.LatencyMS)),
	}
	emitRow(deps, platformlog.KindAICall, parseLevel(base.Level), event)
}

// buildAICallMessage 组一条人能读懂、且不含用户内容的摘要。
func buildAICallMessage(scene, vendor, model string) string {
	parts := make([]string, 0, 3)
	for _, item := range []string{scene, vendor, model} {
		if item != "" {
			parts = append(parts, item)
		}
	}
	if len(parts) == 0 {
		return "AI 调用"
	}
	return fmt.Sprintf("AI 调用（%s）", strings.Join(parts, " / "))
}

// clampToUint32 负数按 0 处理。
//
// 必须显式 clamp：Go 的 int → uint32 是位截断，-1 会变成 4294967295，
// 在后台看到的是"这一次调用用了 42 亿 token"。
func clampToUint32(value int) uint32 {
	if value <= 0 {
		return 0
	}
	if value > int(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(value)
}

// clampToUint8 同上。
func clampToUint8(value int) uint8 {
	if value <= 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

// boolToUint8 把布尔转成 ClickHouse 的 UInt8（0/1）。
func boolToUint8(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}
