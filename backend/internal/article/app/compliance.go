package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/risk"
)

// ComplianceResult 是一次模型合规判定的结论（票 312 / ADR-0033）。
//
// 它是**跨模块的输入形状**：AI 模块判完把结论交回来，文章模块负责落库
// （风险记录表属于文章模块，AI 模块不写别人的表）。
type ComplianceResult struct {
	Verdict       string
	RiskLevel     int
	Categories    []string
	Reasons       []string
	PromptVersion string
	ModelName     string
	CheckedAt     time.Time
}

// SaveComplianceResult 把一次合规判定的结论写进风险记录表。
//
// 覆盖写：一篇文章只保留最近一次的结论，不存历史（ADR-0033 明确不做误判
// 复盘，与评分记录的取舍一致）。
//
// 失败语义：并发写同一篇文章时，后写的覆盖先写的；唯一键保证不会出现两行。
func (s *Service) SaveComplianceResult(ctx context.Context, articleID domain.ArticleID, result ComplianceResult) error {
	if articleID.IsZero() {
		return apperr.New(apperr.CodeArticleNotFound)
	}
	categoriesJSON, err := marshalStrings(result.Categories)
	if err != nil {
		return err
	}
	reasonsJSON, err := marshalStrings(result.Reasons)
	if err != nil {
		return err
	}
	checkedAt := result.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = s.clock.Now()
	}

	record := &risk.Record{
		ArticleID:      articleID,
		Verdict:        risk.Normalize(result.Verdict),
		RiskLevel:      clampRiskLevel(result.RiskLevel),
		CategoriesJSON: categoriesJSON,
		ReasonsJSON:    reasonsJSON,
		PromptVersion:  strings.TrimSpace(result.PromptVersion),
		ModelName:      strings.TrimSpace(result.ModelName),
		CheckedAt:      checkedAt,
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		return r.Risks().Upsert(ctx, record)
	})
}

// ComplianceOf 读一篇文章最近一次的合规结论；没有记录时返回 (nil, nil)。
//
// 后台用它展示"未判定"——这与"判定为干净"是两件事，调用方不要合并展示。
func (s *Service) ComplianceOf(ctx context.Context, articleID domain.ArticleID) (*risk.Record, error) {
	var record *risk.Record
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		record, err = r.Risks().FindByArticle(ctx, articleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// marshalStrings 把字符串切片编成 JSON 数组；nil 时给 "[]" 而不是 "null"，
// 让后台不必区分"空数组"与"字段缺失"。
func marshalStrings(items []string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	trimmed := make([]string, 0, len(items))
	for _, item := range items {
		if v := strings.TrimSpace(item); v != "" {
			trimmed = append(trimmed, v)
		}
	}
	if len(trimmed) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// clampRiskLevel 收敛风险级别到 0–3。
func clampRiskLevel(level int) int {
	if level < 0 {
		return 0
	}
	if level > 3 {
		return 3
	}
	return level
}
