package favorite

import (
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"
)

type FavoriteRequest struct {
	ID       ctype.ID `json:"id"`
	Title    string   `json:"title" binding:"required,min=2,max=32"`
	Cover    string   `json:"cover"`
	Abstract string   `json:"abstract" binding:"required,max=256"`
}

type FavoriteCreateResponse struct {
	ID           ctype.ID `json:"id"`
	UserID       ctype.ID `json:"user_id"`
	Title        string   `json:"title"`
	Cover        string   `json:"cover"`
	Abstract     string   `json:"abstract"`
	IsDefault    bool     `json:"is_default"`
	ArticleCount int      `json:"article_count"`
}

type FavoriteListRequest struct {
	common.PageInfo
	UserID    ctype.ID `form:"user_id"`
	ArticleID ctype.ID `form:"article_id"`
	Type      int8     `form:"type" binding:"required,oneof=1 2 3"` // 1:查自己 2:查别人 3:管理员后台查
}

type FavoriteListResponse struct {
	models.FavoriteModel
	ArticleCount int    `json:"article_count"`
	Nickname     string `json:"nickname,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	HasArticle   bool   `json:"has_article"`
}

type FavoriteArticlesRequest struct {
	common.PageInfo
	FavoriteID ctype.ID `form:"favorite_id" binding:"required"`
}

type FavoriteArticleResponse struct {
	FavoritedAt   time.Time          `json:"favorited_at"`
	ArticleID     ctype.ID           `json:"article_id"`
	Title         string             `json:"title"`
	Abstract      string             `json:"abstract"`
	Cover         string             `json:"cover"`
	ViewCount     int                `json:"view_count"`
	DiggCount     int                `json:"digg_count"`
	CommentCount  int                `json:"comment_count"`
	FavorCount    int                `json:"favor_count"`
	UserNickname  string             `json:"user_nickname"`
	UserAvatar    string             `json:"user_avatar"`
	ArticleStatus enum.ArticleStatus `json:"article_status"`
}

type FavoriteRemovePatchModel struct {
	FavoriteID ctype.ID   `json:"favorite_id" binding:"required"`
	Articles   []ctype.ID `json:"articles" binding:"required"`
}
