package ai_api

import (
	"myblogx/models/ctype"
	"myblogx/service/ai_service/ai_diagnose"
	"myblogx/service/ai_service/ai_metainfo"
	"myblogx/service/ai_service/ai_overwrite"
	"myblogx/service/ai_service/ai_scoring"
	"time"
)

type AIBaseRequest struct {
	Content string `json:"content" binding:"required"`
}

type AIBaseResponse struct {
	Content string `json:"content" binding:"required"`
}

type AIArticleMetaInfoResponse struct {
	Title    string                  `json:"title"`
	Abstract string                  `json:"abstract"`
	Category *ai_metainfo.Metainfos  `json:"category"`
	Tags     []ai_metainfo.Metainfos `json:"tags"`
}

type AIArticleScoringRequest struct {
	Type      int       `json:"type" binding:"required,oneof=1 2 3"`
	ArticleID *ctype.ID `json:"article_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
}

type AIArticleScoringResponse struct {
	HasScore       bool                           `json:"has_score"`
	RecordID       *ctype.ID                      `json:"record_id,omitempty"`
	ArticleID      *ctype.ID                      `json:"article_id,omitempty"`
	AITotalScore   int                            `json:"ai_total_score,omitempty"`
	TotalScore     int                            `json:"total_score,omitempty"`
	ScoreLevel     string                         `json:"score_level,omitempty"`
	ArticleType    string                         `json:"article_type,omitempty"`
	Dimensions     []AIArticleScoreDimension      `json:"dimensions,omitempty"`
	MainIssues     []ai_scoring.ArticleScoreIssue `json:"main_issues,omitempty"`
	OverallComment string                         `json:"overall_comment,omitempty"`
	CreatedAt      *time.Time                     `json:"created_at,omitempty"`
}

type AIArticleScoreDimension struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
}

type AIOverwriteRequest struct {
	Mode          string `json:"mode" binding:"required,oneof=polish grammar_fix style_transform"`
	SelectionText string `json:"selection_text" binding:"required,max=3500"`
	PrefixText    string `json:"prefix_text" binding:"max=800"`
	SuffixText    string `json:"suffix_text" binding:"max=800"`
	ArticleTitle  string `json:"article_title" binding:"required,max=50"`
	TargetStyle   string `json:"target_style" binding:"max=30"`
}

type AIDiagnoseRequest struct {
	SelectionText string `json:"selection_text" binding:"required,max=3500"`
	PrefixText    string `json:"prefix_text" binding:"max=800"`
	SuffixText    string `json:"suffix_text" binding:"max=800"`
	ArticleTitle  string `json:"article_title" binding:"required,max=50"`
}

type AIDiagnoseResponse = ai_diagnose.DiagnoseResponse

func (h AIOverwriteRequest) toServiceRequest() ai_overwrite.RewriteRequest {
	return ai_overwrite.RewriteRequest{
		Mode:          h.Mode,
		SelectionText: h.SelectionText,
		PrefixText:    h.PrefixText,
		SuffixText:    h.SuffixText,
		ArticleTitle:  h.ArticleTitle,
		TargetStyle:   h.TargetStyle,
	}
}

func (h AIDiagnoseRequest) toServiceRequest() ai_diagnose.DiagnoseRequest {
	return ai_diagnose.DiagnoseRequest{
		SelectionText: h.SelectionText,
		PrefixText:    h.PrefixText,
		SuffixText:    h.SuffixText,
		ArticleTitle:  h.ArticleTitle,
	}
}
