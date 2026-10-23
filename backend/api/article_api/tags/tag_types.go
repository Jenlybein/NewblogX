package tags

import (
	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
)

type TagRequest struct {
	ID          ctype.ID `json:"id"`
	Title       string   `json:"title" binding:"required,min=1,max=64"`
	Sort        int      `json:"sort" default:"0"`
	Description string   `json:"description" binding:"max=255"`
	IsEnabled   *bool    `json:"is_enabled"`
}

type TagCreateResponse struct {
	ID          ctype.ID `json:"id"`
	Title       string   `json:"title"`
	Sort        int      `json:"sort"`
	Description string   `json:"description"`
	IsEnabled   bool     `json:"is_enabled"`
}

type TagListRequest struct {
	common.PageInfo
	IsEnabled *bool `form:"is_enabled"`
}

type TagListResponse struct {
	models.TagModel
}
