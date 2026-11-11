package app

import (
	"context"
	"strings"
	"time"
)

// 评论的两层审核（票 359）。
//
// 为什么评论要单独做而不是复用文章那条同步链路：
//
//  1. **机器层是同步的**：词表命中是纯字符串判断，几微秒，命中阻断词直接拒绝，
//     没必要入库再删。
//  2. **AI 层也是同步的，但有硬超时**：评论是短文本，判定一次通常 1–3 秒；
//     超时按"未判定"处理（留在审核中），不按"通过"处理 —— 审核这件事上
//     失败的方向必须是保守的那一边。
//
// 判定顺序也有讲究：先词表后 AI。词表命中阻断词时不必花一次模型调用；
// 词表命中观察词时也不该让 AI 的 clean 结论把人工复核挡回去。

// Verdict 是 AI 合规结论的档位（与文章侧同构）。
type Verdict string

const (
	// VerdictClean 没发现问题。
	VerdictClean Verdict = "clean"
	// VerdictSuspect 可疑，交人工。
	VerdictSuspect Verdict = "suspect"
	// VerdictViolation 明确违规。
	VerdictViolation Verdict = "violation"
)

// ComplianceInput 是一次 AI 判定的输入。
type ComplianceInput struct {
	// ArticleTitle 是评论所属文章的标题：给模型一点语境，
	// 避免把技术讨论里的"破解"误判成违法内容。
	ArticleTitle string
	// Content 是评论正文。
	Content string
	// MaxTokens 是本次判定的输出上限。
	MaxTokens int
}

// ComplianceResult 是 AI 判定的结论。
type ComplianceResult struct {
	Verdict Verdict
	// Categories 命中的类别（后台展示用）。
	Categories []string
	// Reason 是最主要的一条依据（只写日志，不给作者 —— 给了等于教人绕过）。
	Reason string
}

// ComplianceChecker 是 AI 合规判定的端口。
//
// 由装配层接到 AI 模块：评论模块不认识模型、不认识供应商，只认识这个接口。
type ComplianceChecker interface {
	CheckComment(ctx context.Context, in ComplianceInput) (*ComplianceResult, error)
}

// ModerationPolicy 是评论审核的运行期策略快照。
type ModerationPolicy struct {
	// BlockWords 命中即拒绝（不入库）。
	BlockWords []string
	// WatchWords 命中即留待人工（即使开了免审核）。
	WatchWords []string
	// AIEnabled 是否做 AI 判定。
	AIEnabled bool
	// AIMaxTokens 单次判定的输出上限。
	AIMaxTokens int
	// AITimeout AI 判定的硬超时。
	AITimeout time.Duration
}

// defaultAIMaxTokens 与 conf 的默认值保持一致；两处都写是因为 conf 那一份
// 可能被 yaml 缺省绕过（结构体零值），这里再兜一次底。
const defaultAIMaxTokens = 1000

// defaultAITimeout 是 AI 判定的兜底超时。
const defaultAITimeout = 8 * time.Second

// normalize 补齐零值，避免"配置漏填"变成"没有超时"。
func (p ModerationPolicy) normalize() ModerationPolicy {
	if p.AIMaxTokens <= 0 {
		p.AIMaxTokens = defaultAIMaxTokens
	}
	if p.AITimeout <= 0 {
		p.AITimeout = defaultAITimeout
	}
	return p
}

// matchWords 返回文本命中的词（小写比较，词本身也小写化）。
//
// 用包含判断而不是分词：中文没有空格，分词器引入的误差比收益大；
// 词表由运营维护，本来就是"宁可多拦"的口径。
func matchWords(text string, words []string) []string {
	if text == "" || len(words) == 0 {
		return nil
	}
	lower := strings.ToLower(text)
	var hits []string
	for _, word := range words {
		w := strings.ToLower(strings.TrimSpace(word))
		if w == "" {
			continue
		}
		if strings.Contains(lower, w) {
			hits = append(hits, word)
		}
	}
	return hits
}

// moderationPolicy 返回当前的审核策略快照。
//
// 没接入审核时返回零值（词表空、AI 关闭）：零值让 matchWords 立刻返回空、
// 让 AI 分支不进入，等于"与接入前行为一致"，而不是"悄悄拒绝所有评论"。
func (s *Service) moderationPolicy() ModerationPolicy {
	if s.moderation == nil {
		return ModerationPolicy{}
	}
	return s.moderation().normalize()
}

// passAI 跑一次 AI 判定，返回"是否允许直接发布"。
//
// 三个分支的方向都是保守的：
//   - 判定为 clean → 发布；
//   - 判定为 suspect / violation → 留待人工；
//   - 判定失败或超时 → 留待人工（**不能**按 clean 处理：模型不可用时放行，
//     等于把审核开关关掉而没人知道）。
func (s *Service) passAI(ctx context.Context, policy ModerationPolicy, articleTitle, content string, commentID uint64) bool {
	checkCtx, cancel := context.WithTimeout(ctx, policy.AITimeout)
	defer cancel()

	result, err := s.compliance.CheckComment(checkCtx, ComplianceInput{
		ArticleTitle: articleTitle,
		Content:      content,
		MaxTokens:    policy.AIMaxTokens,
	})
	if err != nil {
		s.logError("评论 AI 判定失败，留待人工: comment=%d err=%v", commentID, err)
		return false
	}
	if result == nil {
		s.logError("评论 AI 判定返回空结论，留待人工: comment=%d", commentID)
		return false
	}
	if result.Verdict == VerdictClean {
		return true
	}
	s.logError("评论 AI 判定为 %s，留待人工: comment=%d categories=%v reason=%s",
		result.Verdict, commentID, result.Categories, result.Reason)
	return false
}