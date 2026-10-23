package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"
)

type ArticleReviewTaskStatus string

const (
	ArticleReviewTaskPending  ArticleReviewTaskStatus = "pending"
	ArticleReviewTaskApproved ArticleReviewTaskStatus = "approved"
	ArticleReviewTaskRejected ArticleReviewTaskStatus = "rejected"
	ArticleReviewTaskCanceled ArticleReviewTaskStatus = "canceled"
)

type ArticleReviewTaskStage string

const (
	ArticleReviewTaskStageManual ArticleReviewTaskStage = "manual"
)

type ArticleReviewTaskSource string

const (
	ArticleReviewTaskSourceCreate   ArticleReviewTaskSource = "create"
	ArticleReviewTaskSourceEdit     ArticleReviewTaskSource = "edit"
	ArticleReviewTaskSourceResubmit ArticleReviewTaskSource = "resubmit"
)

type ArticleReviewTaskModel struct {
	Model
	ArticleID            ctype.ID                `gorm:"index;not null" json:"article_id"`
	AuthorID             ctype.ID                `gorm:"index;not null" json:"author_id"`
	ArticleTitle         string                  `gorm:"size:256;not null" json:"article_title"`
	AuthorName           string                  `gorm:"size:32;not null" json:"author_name"`
	ArticlePublishStatus enum.ArticleStatus      `gorm:"default:0;index" json:"publish_status"`
	Stage                ArticleReviewTaskStage  `gorm:"type:varchar(16);not null;default:'manual'" json:"stage"`
	Source               ArticleReviewTaskSource `gorm:"type:varchar(16);not null" json:"source"`
	Status               ArticleReviewTaskStatus `gorm:"type:varchar(16);not null;default:'pending';index" json:"status"`
	Reason               string                  `gorm:"size:500" json:"reason"`
	ReviewedAt           *time.Time              `json:"reviewed_at"`
	ReviewedBy           *ctype.ID               `gorm:"index" json:"reviewed_by"`
}

type ArticleReviewLogModel struct {
	Model
	TaskID     ctype.ID `gorm:"index;not null" json:"task_id"`
	ArticleID  ctype.ID `gorm:"index;not null" json:"article_id"`
	OperatorID ctype.ID `gorm:"index;not null" json:"operator_id"`
	Action     string   `gorm:"size:32;not null" json:"action"`
	FromStatus string   `gorm:"size:32" json:"from_status"`
	ToStatus   string   `gorm:"size:32" json:"to_status"`
	Reason     string   `gorm:"size:500" json:"reason"`
}
