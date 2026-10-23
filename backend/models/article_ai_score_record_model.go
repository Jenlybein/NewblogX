package models

import (
	"myblogx/models/ctype"
	"time"
)

// ArticleAIScoreRecordModel 记录文章 AI 质量评分结果。
// 该表保留每次评分的快照结果，便于作者回看历史评分变化，而不是只覆盖一份最新分数。
type ArticleAIScoreRecordModel struct {
	Model
	ArticleID                ctype.ID   `gorm:"index;not null" json:"article_id"`
	UserID                   ctype.ID   `gorm:"index;not null" json:"user_id"`
	TitleSnapshot            string     `gorm:"size:256;not null" json:"title_snapshot"`
	ContentHash              string     `gorm:"size:64;index;not null" json:"content_hash"`
	ContentLength            int        `gorm:"default:0" json:"content_length"`
	ArticleUpdatedAtSnapshot *time.Time `json:"article_updated_at_snapshot"`
	AITotalScore             int        `gorm:"default:0" json:"ai_total_score"`
	TotalScore               int        `gorm:"default:0;index" json:"total_score"`
	ScoreLevel               string     `gorm:"size:32" json:"score_level"`
	ArticleType              string     `gorm:"size:32" json:"article_type"`
	DimensionsJSON           string     `gorm:"type:longtext;not null" json:"dimensions_json"`
	MainIssuesJSON           string     `gorm:"type:longtext;not null" json:"main_issues_json"`
	OverallComment           string     `gorm:"type:longtext" json:"overall_comment"`
	Provider                 string     `gorm:"size:32" json:"provider"`
	ModelName                string     `gorm:"size:64" json:"model_name"`
	PromptVersion            string     `gorm:"size:32" json:"prompt_version"`
}
