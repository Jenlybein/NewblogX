package view_history

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"time"
)

type ArticleViewHistoryRequest struct {
	common.PageInfo
	UserID ctype.ID `form:"user_id"`
	Type   int8     `form:"type" binding:"required,oneof=1 2"` // 1: 自己的浏览记录 2: 其他人的浏览记录
}

type ArticleViewHistoryResponse struct {
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
	Cover     string    `json:"cover"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	UserID    ctype.ID  `json:"user_id"`
	ArticleID ctype.ID  `json:"article_id"`
}
