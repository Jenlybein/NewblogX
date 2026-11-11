// Package moderation 是投稿的**合规判定**（票 312 / ADR-0033）。
//
// 它判断的问题与 scoring 完全不同：scoring 回答"写得好不好"（结构、清晰度、
// 语言），这里回答"能不能发"（违法交易、诈骗、广告引流等）。两者可能矛盾
// （评分 82 但命中违规），矛盾时以本包的结论为准。
//
// 判定规则本身来自运行时策略文本（skills/moderation/SKILL.md），由调用方
// 拼进提示词；本包只负责"把文章的标题与正文交给模型、把结论解析成结构"。
package moderation

import (
	"context"
	"fmt"
	"strings"

	"myblogx/conf"
	"myblogx/internal/ai"
)

// Verdict 是合规结论的档位。
//
// 三档而不是布尔：布尔会把"没发现问题"与"明显违规"之间的中间地带压成
// 二选一，而审核员真正需要的是"这条要不要先看"。
type Verdict string

const (
	// VerdictClean 没发现问题。
	VerdictClean Verdict = "clean"
	// VerdictSuspect 有可疑之处，但不足以定性，交人工判断。
	VerdictSuspect Verdict = "suspect"
	// VerdictViolation 明确违规。
	VerdictViolation Verdict = "violation"
)

// Valid 判断档位取值是否合法。
func (v Verdict) Valid() bool {
	switch v {
	case VerdictClean, VerdictSuspect, VerdictViolation:
		return true
	}
	return false
}

// NormalizeVerdict 把模型给的原始值收敛到三档。
//
// 模型偶尔会给出 "ok" "safe" "high" 这类同义词；认不出来的一律按 suspect：
// 归到 clean 等于静默放行，归到 violation 等于静默拒绝，两者都比"交人看"更糟。
func NormalizeVerdict(raw string) Verdict {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(VerdictClean), "ok", "safe", "pass":
		return VerdictClean
	case string(VerdictViolation), "high", "block", "violate":
		return VerdictViolation
	default:
		return VerdictSuspect
	}
}

// Result 是一次合规判定的结论。
type Result struct {
	Verdict Verdict
	// RiskLevel 是 0–3 的风险级别，用于审核列表排序。
	RiskLevel int
	// Categories 是命中的类别（如"广告引流""违法交易"），用于后台展示。
	Categories []string
	// Reasons 是逐条依据；只给管理员看，不给作者（等于绕过指南）。
	Reasons []string
	// ModelName 是这次判定用的模型，随记录落库便于回溯。
	ModelName string
}

// CheckRequest 是一次合规判定的输入。
type CheckRequest struct {
	Title   string
	Content string
	// Policy 是运行时策略文本正文（skills/moderation/SKILL.md）；空串表示
	// 站点没有绑定策略文本，此时只用内置的判定要求。
	Policy string
	// MaxTokens 是**本次判定**允许的输出长度上限；<=0 用供应商配置里的值。
	//
	// 合规结论只是一个短 JSON，给足 1000 token 已经宽裕；按次限制能把审核成本
	// 钉在一个可预期的量级，而不必为了审核把供应商的全局 max_tokens 调小
	// （那会连带截断正常对话）。
	MaxTokens int
}

// 文章正文送入模型的上限。
//
// 合规判定看的是"有没有违规表述"，不需要读完全文；截断反而能降低单次成本。
// 取 20000 字符与评分分块阈值同量级，超出部分省略并明确告知模型。
const maxContentChars = 20000

// checkResponse 是模型的输出结构。
type checkResponse struct {
	Verdict    string   `json:"verdict"`
	RiskLevel  int      `json:"risk_level"`
	Categories []string `json:"categories"`
	Reasons    []string `json:"reasons"`
}

// CheckCompliance 调模型做一次合规判定。
//
// 返回 error 表示**没拿到可用结论**（网络错、非 JSON、模型拒答等），由调用方
// 按"未判定"处理——注意这与"判定为干净"是两件事，绝不能混。
func CheckCompliance(ctx context.Context, aiConf conf.AI, req CheckRequest) (*Result, error) {
	content := strings.TrimSpace(req.Content)
	if len([]rune(content)) > maxContentChars {
		content = truncateRunes(content, maxContentChars) + "\n\n（正文以下内容已省略）"
	}

	messages := []ai.Message{
		{Role: "system", Content: systemPrompt(req.Policy)},
		{Role: "user", Content: userPrompt(req.Title, content)},
	}

	reply, err := ai.ChatWithLimit(ctx, aiConf, messages, req.MaxTokens)
	if err != nil {
		return nil, err
	}

	var parsed checkResponse
	if err := ai.UnmarshalJSONBlock(reply, &parsed); err != nil {
		return nil, fmt.Errorf("合规判定结果不是有效 JSON: %w", err)
	}

	level := parsed.RiskLevel
	if level < 0 {
		level = 0
	}
	if level > 3 {
		level = 3
	}

	return &Result{
		Verdict:    NormalizeVerdict(parsed.Verdict),
		RiskLevel:  level,
		Categories: trimAll(parsed.Categories),
		Reasons:    trimAll(parsed.Reasons),
		ModelName:  activeModelName(aiConf),
	}, nil
}

// activeModelName 取本次使用的模型名（Active 由装配层现填，可能为 nil）。
func activeModelName(aiConf conf.AI) string {
	if aiConf.Active == nil {
		return ""
	}
	return aiConf.Active.Model
}

// systemPrompt 组合"运行时策略文本 + 内置判定要求"。
//
// 策略文本（skills/moderation/SKILL.md）由运营维护，解释"这个站点在意什么"；
// 内置这段只规定**输出契约**，保证解析稳定。两者分开写，改文案不会改解析。
func systemPrompt(policy string) string {
	var b strings.Builder
	b.WriteString("你是一名中文内容合规审查助手，只负责判断投稿是否涉及违规内容。\n\n")
	if trimmed := strings.TrimSpace(policy); trimmed != "" {
		b.WriteString("本站在意的规则如下（运营维护，优先遵守）：\n")
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString(`请严格遵守以下输出要求：
1. 只输出一个 JSON 对象，不要输出解释、标题、前后缀或 Markdown 代码块。
2. verdict 只能是以下三个值之一：
   - "clean"：没有发现问题；
   - "suspect"：有可疑之处但不足以定性，应交人工判断；
   - "violation"：明确违规（违法交易、诈骗、色情、暴力煽动、广告引流等）。
3. risk_level 是 0–3 的整数：0 = 无风险，1 = 低，2 = 中，3 = 高。
4. categories 是命中的类别数组，没有命中时给空数组。
5. reasons 是逐条判断依据的数组，每条一句话；没有命中时给空数组。
6. 宁可给 "suspect" 交人判断，也不要为了显得严格而轻易给 "violation"：
   自动拦截会落到作者身上，而他没有申诉渠道。
7. 不要因为文章讨论"违法/色情/诈骗"这类**话题**就判违规——只有陈述、
   宣传或引导才可能是违规。科普与讨论不算。
`)
	return b.String()
}

// userPrompt 给出待判定的文章。
func userPrompt(title, content string) string {
	return fmt.Sprintf("文章标题：\n%s\n\n正文：\n%s", strings.TrimSpace(title), content)
}

// aiConf 是 conf.AI 的类型别名占位，用于让本包不直接依赖 Active 的字段形状。
//
// 说明：ActiveModelName 通过下面的小工具取值，避免在本包内复制一份模型名规则。

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func trimAll(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
