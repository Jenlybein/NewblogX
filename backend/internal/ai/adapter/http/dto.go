package http

import (
	"time"

	"myblogx/internal/ai/diagnose"
	"myblogx/internal/ai/metainfo"
	"myblogx/internal/ai/scoring"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

// ── 请求 ───────────────────────────────────────────────────────

// BaseRequest 是只带一段正文的请求（元信息、AI 搜索都用它）。
type BaseRequest struct {
	Content string `json:"content" binding:"required"`
}

// ScoringRequest 对应 POST /ai/scoring/article。
type ScoringRequest struct {
	Type      int       `json:"type" binding:"required,oneof=1 2 3"`
	ArticleID *ctype.ID `json:"article_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
}

// OverwriteRequest 对应 POST /ai/overwrite。
type OverwriteRequest struct {
	Mode          string `json:"mode" binding:"required,oneof=polish grammar_fix style_transform"`
	SelectionText string `json:"selection_text" binding:"required,max=3500"`
	PrefixText    string `json:"prefix_text" binding:"max=800"`
	SuffixText    string `json:"suffix_text" binding:"max=800"`
	ArticleTitle  string `json:"article_title" binding:"required,max=50"`
	TargetStyle   string `json:"target_style" binding:"max=30"`
}

// DiagnoseRequest 对应 POST /ai/diagnose。
type DiagnoseRequest struct {
	SelectionText string `json:"selection_text" binding:"required,max=3500"`
	PrefixText    string `json:"prefix_text" binding:"max=800"`
	SuffixText    string `json:"suffix_text" binding:"max=800"`
	ArticleTitle  string `json:"article_title" binding:"required,max=50"`
}

// ── 响应 ───────────────────────────────────────────────────────

// BaseResponse 是 SSE 流的单条内容。
type BaseResponse struct {
	Content string `json:"content" binding:"required"`
}

// ScoreDimension 是一个评分维度。
type ScoreDimension struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
}

// ScoringResponse 是评分接口的返回。
type ScoringResponse struct {
	HasScore       bool                        `json:"has_score"`
	RecordID       *ctype.ID                   `json:"record_id,omitempty"`
	ArticleID      *ctype.ID                   `json:"article_id,omitempty"`
	AITotalScore   int                         `json:"ai_total_score,omitempty"`
	TotalScore     int                         `json:"total_score,omitempty"`
	ScoreLevel     string                      `json:"score_level,omitempty"`
	ArticleType    string                      `json:"article_type,omitempty"`
	Dimensions     []ScoreDimension            `json:"dimensions,omitempty"`
	MainIssues     []scoring.ArticleScoreIssue `json:"main_issues,omitempty"`
	OverallComment string                      `json:"overall_comment,omitempty"`
	CreatedAt      *time.Time                  `json:"created_at,omitempty"`
}

// DiagnoseResponse 是诊断结果（结构由 diagnose 包定义）。
type DiagnoseResponse = diagnose.DiagnoseResponse

// MetaInfoResponse 是元信息生成结果。
type MetaInfoResponse = metainfo.MetainfoResponse

// ── 绑定助手 ───────────────────────────────────────────────────

// bindJSON 取中间件已经绑定好的请求体。
func bindJSON[T any](c *gin.Context) (T, bool) {
	var zero T
	if c.IsAborted() {
		return zero, false
	}
	raw, ok := c.Get("requestJson")
	if !ok {
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}
