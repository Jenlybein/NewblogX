// Package risk 是文章的**合规风险记录**子域（票 312 / ADR-0024）。
//
// 它只回答一件事：这篇文章最近一次模型合规判定的结论是什么。
// 文章表与审核任务表都不加列（零 DDL 硬约束），结论只能落在这张经 ADR
// 指名的新表上；一篇文章一行、随提交覆盖、不存历史。
package risk

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// Verdict 是合规结论的档位（与 AI 模块 moderation 包的三档一致）。
type Verdict string

const (
	// VerdictClean 没发现问题。
	VerdictClean Verdict = "clean"
	// VerdictSuspect 有可疑之处，交人工判断。
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

// Normalize 把未知档位收敛到 suspect。
//
// 归到 clean 等于静默放行，归到 violation 等于静默拒绝；两者都比"交人看"更糟。
func Normalize(raw string) Verdict {
	switch Verdict(raw) {
	case VerdictClean:
		return VerdictClean
	case VerdictViolation:
		return VerdictViolation
	default:
		return VerdictSuspect
	}
}

// Record 是一篇文章的合规判定记录。
type Record struct {
	ArticleID domain.ArticleID
	Verdict   Verdict
	// RiskLevel 是 0–3 的风险级别，用于审核列表排序。
	RiskLevel int
	// CategoriesJSON / ReasonsJSON 是命中的类别与依据（JSON 数组）。
	CategoriesJSON string
	ReasonsJSON    string
	// PromptVersion / ModelName 是判定时的策略版本与模型，用于回溯。
	PromptVersion string
	ModelName     string
	CheckedAt     time.Time
}

// Repository 是风险记录的持久化契约。
type Repository interface {
	// Upsert 写入或覆盖一篇文章的结论（一篇文章只有一行）。
	Upsert(ctx context.Context, record *Record) error
	// FindByArticle 读一篇文章的结论；没有记录时返回 (nil, nil)。
	FindByArticle(ctx context.Context, articleID domain.ArticleID) (*Record, error)
	// ListByArticles 批量读结论，键为文章 ID；缺失的文章不出现在结果里。
	ListByArticles(ctx context.Context, articleIDs []domain.ArticleID) (map[ctype.ID]*Record, error)
}
