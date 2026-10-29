package llm

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

// CallUsage 是一次逻辑调用的用量与耗时。
//
// 为什么按"一次逻辑调用"汇总，而不是每次上游请求一条：Chat 有 2 次重试
// （defaultMaxRetries），逐次上报会把后台的"调用次数"放大成 3 倍，
// 与用户感知的"我用了几次 AI"对不上。真实的请求次数记在 Attempts 里。
type CallUsage struct {
	Model string
	Vendor string

	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	ReasoningTokens  int

	// Attempts 是实际发往上游的次数（含重试）。0 表示压根没发出去
	// （参数为空、模型客户端构造失败），上报方据此区分"调用失败"和"没调用"。
	Attempts int
	Duration time.Duration

	// Available 表示上游到底回没回用量。
	//
	// 必须和"两个 token 都是 0"分开：自建 OpenAI 兼容网关（vLLM / one-api /
	// ollama）普遍忽略 stream_options.include_usage，永远不回用量；把它记成
	// "用了 0 token"会让人得出"这个模型不要钱"的错误结论。
	Available bool
}

// Total 返回用于统计的总量。
//
// 优先上游给的 TotalTokens；个别网关只回 prompt / completion 而不回 total，
// 这时用两者之和兜底，否则统计里这批数据会凭空消失。
func (u CallUsage) Total() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.PromptTokens + u.CompletionTokens
}

// merge 把一份上游返回的用量并进来，非零字段覆盖。
//
// 为什么不是"拿最后一次整体覆盖"：各家语义并不统一 ——
//   - gemini 每个 chunk 都塞一份累计的 UsageMetadata；
//   - claude 只在 MessageDeltaEvent 里给累计值；
//   - openai 系在流结束时补发一条 Content 为空、只挂 usage 的消息。
//
// 取并集才能兼容"只有部分字段有值"的供应商；累计语义下最后一次覆盖即最终值，
// 与并集结果一致。
func (u *CallUsage) merge(tu *schema.TokenUsage) {
	if tu == nil {
		return
	}
	u.Available = true
	// 覆盖而不是累加：数值是累计的，累加会翻倍。
	if tu.PromptTokens > 0 {
		u.PromptTokens = tu.PromptTokens
	}
	if tu.CompletionTokens > 0 {
		u.CompletionTokens = tu.CompletionTokens
	}
	if tu.TotalTokens > 0 {
		u.TotalTokens = tu.TotalTokens
	}
	if tu.PromptTokenDetails.CachedTokens > 0 {
		u.CachedTokens = tu.PromptTokenDetails.CachedTokens
	}
	if tu.CompletionTokensDetails.ReasoningTokens > 0 {
		u.ReasoningTokens = tu.CompletionTokensDetails.ReasoningTokens
	}
}

// mergeMessage 从 eino 的消息上取用量。ResponseMeta 与 Usage 都可能是 nil。
func (u *CallUsage) mergeMessage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil {
		return
	}
	u.merge(msg.ResponseMeta.Usage)
}
