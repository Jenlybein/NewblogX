package article_api

import (
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"
)

type ArticleCreateRequest struct {
	Title            string                       `json:"title" binding:"required"`
	Abstract         string                       `json:"abstract"`
	Content          string                       `json:"content" binding:"required"`
	CategoryID       *ctype.ID                    `json:"category_id"`
	TagIDs           []ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                    `json:"cover_image_id"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	Status           enum.ArticleStatus           `json:"status" binding:"required,oneof=1 2"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden"`
}

type ArticleCreateResponse struct {
	ID               ctype.ID                     `json:"id"`
	Title            string                       `json:"title"`
	CategoryID       *ctype.ID                    `json:"category_id"`
	TagIDs           []ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                    `json:"cover_image_id,omitempty"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	Status           enum.ArticleStatus           `json:"status"`
	PublishStatus    enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status"`
}

type ArticleDetailResponse struct {
	ID                  ctype.ID                     `gorm:"primaryKey" json:"id"`
	CreatedAt           time.Time                    `json:"created_at"`
	UpdatedAt           time.Time                    `json:"updated_at"`
	Title               string                       `json:"title"`
	Abstract            string                       `json:"abstract"`
	Content             string                       `json:"content"`
	CategoryID          *ctype.ID                    `json:"category_id"`
	TagIDs              []ctype.ID                   `json:"tag_ids"`
	CoverImageID        *ctype.ID                    `json:"cover_image_id,omitempty"`
	Cover               string                       `json:"cover"`
	ViewCount           int                          `json:"view_count"`
	DiggCount           int                          `json:"digg_count"`
	CommentCount        int                          `json:"comment_count"`
	FavorCount          int                          `json:"favor_count"`
	CommentsToggle      bool                         `json:"comments_toggle"`
	Status              enum.ArticleStatus           `json:"status"`
	PublishStatus       enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus    enum.ArticleVisibilityStatus `json:"visibility_status"`
	Tags                []string                     `json:"tags"`
	AuthorID            ctype.ID                     `json:"author_id"`
	AuthorAvatarImageID *ctype.ID                    `json:"author_avatar_image_id,omitempty"`
	AuthorAvatar        string                       `json:"author_avatar"`
	AuthorAbstract      string                       `json:"author_abstract"`
	AuthorCreatedAt     time.Time                    `json:"author_created_time"`
	AuthorNickname      string                       `json:"author_name"`
	AuthorUsername      string                       `json:"author_username"`
	CategoryName        string                       `json:"category_name"`
	IsDigg              bool                         `json:"is_digg"`
	IsFavor             bool                         `json:"is_favor"`
}

type ArticleExamineRequest struct {
	Status enum.ArticleStatus `json:"status" binding:"required,oneof=3 4"`
	Reason string             `json:"reason"`
}

type ArticleFavoriteRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
	FavorID   ctype.ID `json:"favor_id"`
}

type ArticleUpdateRequest struct {
	Title            *string                       `json:"title"`
	Abstract         *string                       `json:"abstract"`
	Content          *string                       `json:"content"`
	CategoryID       *ctype.ID                     `json:"category_id"`
	TagIDs           *[]ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                     `json:"cover_image_id"`
	CommentsToggle   *bool                         `json:"comments_toggle"`
	Status           *enum.ArticleStatus           `json:"status" binding:"omitempty,oneof=1 2"`
	VisibilityStatus *enum.ArticleVisibilityStatus `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden"`
}

type ArticleReviewTaskListRequest struct {
	Page   int                            `form:"page"`
	Limit  int                            `form:"limit"`
	Status models.ArticleReviewTaskStatus `form:"status" binding:"omitempty,oneof=pending approved rejected canceled"`
}

type ArticleReviewTaskItem struct {
	ID            ctype.ID                       `json:"id"`
	ArticleID     ctype.ID                       `json:"article_id"`
	AuthorID      ctype.ID                       `json:"author_id"`
	ArticleTitle  string                         `json:"article_title"`
	AuthorName    string                         `json:"author_name"`
	PublishStatus enum.ArticleStatus             `json:"publish_status"`
	Stage         models.ArticleReviewTaskStage  `json:"stage"`
	Source        models.ArticleReviewTaskSource `json:"source"`
	Status        models.ArticleReviewTaskStatus `json:"status"`
	Reason        string                         `json:"reason"`
	CreatedAt     time.Time                      `json:"created_at"`
	ReviewedAt    *time.Time                     `json:"reviewed_at"`
	ReviewedBy    *ctype.ID                      `json:"reviewed_by"`
}

type ArticleReviewTaskListResponse struct {
	Count int64                   `json:"count"`
	List  []ArticleReviewTaskItem `json:"list"`
}

type ArticleReviewHandleRequest struct {
	Status enum.ArticleStatus `json:"status" binding:"required,oneof=3 4"`
	Reason string             `json:"reason"`
}

type ArticleAdminVisibilityURI struct {
	ID         ctype.ID `uri:"id" binding:"required"`
	Visibility string   `uri:"visibility" binding:"required,oneof=hide show"`
}

type ArticleViewCountRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
}
