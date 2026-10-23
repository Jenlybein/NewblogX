package category

import (
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
)

type CategoryRequest struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title" binding:"required,min=2,max=20"`
}

type CategoryCreateResponse struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

type CategoryListRequest struct {
	common.PageInfo
	UserID ctype.ID `form:"user_id"`
	Type   int8     `form:"type" binding:"required,oneof=1 2 3"` // 1:查自己 2:查别人 3:管理员后台查
}

type CategoryOptionsRequest struct {
	UserID ctype.ID `form:"user_id" binding:"required"`
}

type CategoryListResponse struct {
	models.CategoryModel
	ArticleCount int    `json:"article_count"`
	Nickname     string `json:"nickname,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
}
