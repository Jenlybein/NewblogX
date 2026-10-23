package global_notif_api

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"myblogx/models/enum/global_notif_enum"
	"time"
)

type GlobalNotifCreateRequest struct {
	ExpireTime      *time.Time             `json:"expire_time"`
	UserVisibleRule global_notif_enum.Type `json:"user_visible_rule"`
	Title           string                 `json:"title" binding:"required"`
	Content         string                 `json:"content" binding:"required"`
	Icon            string                 `json:"icon"`
	Href            string                 `json:"href"`
}

type GlobalNotifCreateResponse struct {
	ID              ctype.ID               `json:"id"`
	ExpireTime      time.Time              `json:"expire_time"`
	UserVisibleRule global_notif_enum.Type `json:"user_visible_rule"`
	Title           string                 `json:"title"`
	Content         string                 `json:"content"`
	Icon            string                 `json:"icon"`
	Href            string                 `json:"href"`
}

type GlobalNotifListRequest struct {
	common.PageInfo
	Type int8 `form:"type" binding:"required,oneof=1 2"`
	// 1:用户查全局消息 2:管理员查全局消息
}

type GlobalNotifListResponse struct {
	ID       ctype.ID  `json:"id"`
	CreateAt time.Time `json:"create_at"`
	Title    string    `json:"title"`   // 通知标题
	Icon     string    `json:"icon"`    // 通知图标
	Content  string    `json:"content"` // 通知内容
	Href     string    `json:"herf"`    // 通知链接
	IsRead   bool      `json:"is_read"`
}
