package top

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"
)

type ArticleTopSetRequest struct {
	ArticleID ctype.ID `json:"article_id"`
	Type      int      `json:"type" binding:"required,oneof=1 2"`
}

type ArticleTopListRequest struct {
	Type   int      `json:"type" form:"type" binding:"required,oneof=1 2"`
	UserID ctype.ID `json:"user_id" form:"user_id"`
}

type ArticleTopListResponse struct {
	ID             ctype.ID           `json:"id"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Title          string             `json:"title"`
	Abstract       string             `json:"abstract"`
	Cover          string             `json:"cover"`
	ViewCount      int                `json:"view_count"`
	DiggCount      int                `json:"digg_count"`
	CommentCount   int                `json:"comment_count"`
	FavorCount     int                `json:"favor_count"`
	CommentsToggle bool               `json:"comments_toggle"`
	Status         enum.ArticleStatus `json:"status"`
	Tags           []string           `json:"tags"`
	UserTop        bool               `json:"user_top"`
	AdminTop       bool               `json:"admin_top"`
	CategoryTitle  string             `json:"category_title"`
	UserNickname   string             `json:"user_nickname"`
	UserAvatar     string             `json:"user_avatar"`
}
