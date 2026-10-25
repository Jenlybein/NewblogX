package http

import (
	"time"

	"myblogx/common/res"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// ── 请求 ───────────────────────────────────────────────────────

// GlobalListRequest 对应 GET /global_notif 的查询参数。
//
// 保留 key/order 是因为旧接口把它们透传给了通用列表查询：
// key 走标题/内容模糊匹配，order 传非空会直接报"排序字段错误"。
type GlobalListRequest struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"`
	Type  int8   `form:"type" binding:"required,oneof=1 2"`
}

// GlobalCreateRequest 对应 POST /global_notif。
type GlobalCreateRequest struct {
	ExpireTime      *time.Time `json:"expire_time"`
	UserVisibleRule int8       `json:"user_visible_rule"`
	Title           string     `json:"title" binding:"required"`
	Content         string     `json:"content" binding:"required"`
	Icon            string     `json:"icon"`
	Href            string     `json:"href"`
}

// IDListRequest 是批量操作的 ID 列表。
type IDListRequest struct {
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

// MessageListRequest 对应 GET /sitemsg 的查询参数。
type MessageListRequest struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"`
	T     int8   `form:"t" binding:"required,oneof=1 2 3"`
}

// MessageSelectRequest 对应 POST/DELETE /sitemsg：按 id 或按分组 t 操作。
type MessageSelectRequest struct {
	ID ctype.ID `json:"id"`
	T  int8     `json:"t" binding:"omitempty,oneof=1 2 3"`
}

// ConfRequest 对应 GET/PUT /sitemsg/conf。
type ConfRequest struct {
	DiggNoticeEnabled        bool `json:"digg_notice_enabled"`
	CommentNoticeEnabled     bool `json:"comment_notice_enabled"`
	FavorNoticeEnabled       bool `json:"favor_notice_enabled"`
	PrivateChatNoticeEnabled bool `json:"private_chat_notice_enabled"`
}

// ── 响应 ───────────────────────────────────────────────────────

// GlobalListResponse 是公告列表项。
//
// `herf` 这个拼写是历史契约的一部分，前端按它读，不能"顺手修正"。
type GlobalListResponse struct {
	ID       ctype.ID  `json:"id"`
	CreateAt time.Time `json:"create_at"`
	Title    string    `json:"title"`
	Icon     string    `json:"icon"`
	Content  string    `json:"content"`
	Href     string    `json:"herf"`
	IsRead   bool      `json:"is_read"`
}

// GlobalCreateResponse 是创建公告的返回。
type GlobalCreateResponse struct {
	ID              ctype.ID  `json:"id"`
	ExpireTime      time.Time `json:"expire_time"`
	UserVisibleRule int8      `json:"user_visible_rule"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Icon            string    `json:"icon"`
	Href            string    `json:"href"`
}

// MessageItem 是站内信列表项。
//
// 旧接口直接返回 GORM 模型，字段由模型的 json tag 决定；
// 这里逐字段对齐，包括 id 序列化成字符串（ctype.ID 的 MarshalJSON）。
type MessageItem struct {
	ID                 ctype.ID   `json:"id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at"`
	Type               int8       `json:"type"`
	ReceiverID         ctype.ID   `json:"receiver_id"`
	ActionUserID       *ctype.ID  `json:"action_user_id"`
	ActionUserNickname *string    `json:"action_user_nickname"`
	ActionUserAvatar   *string    `json:"action_user_avatar"`
	Content            string     `json:"content"`
	ArticleID          ctype.ID   `json:"article_id"`
	CommentID          ctype.ID   `json:"comment_id"`
	ArticleTitle       string     `json:"article_title"`
	LinkTitle          string     `json:"link_title"`
	LinkHerf           string     `json:"link_herf"`
	IsRead             bool       `json:"is_read"`
	ReadAt             *time.Time `json:"read_at"`
}

// UnreadResponse 是消息中心角标。
type UnreadResponse struct {
	CommentMsgCount   int `json:"comment_msg_count"`
	DiggFavorMsgCount int `json:"digg_favor_msg_count"`
	PrivateMsgCount   int `json:"private_msg_count"`
	SystemMsgCount    int `json:"system_msg_count"`
	GlobalMsgCount    int `json:"global_msg_count"`
}

// ── 绑定助手 ───────────────────────────────────────────────────

// bindJSON 取中间件已经绑定好的请求体。
func bindJSON[T any](c *gin.Context) (T, bool) {
	var zero T
	if c.IsAborted() {
		return zero, false
	}
	raw, ok := c.Get("requestJson")
	if !ok {
		res.FailWithMsg("请求参数错误", c)
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		res.FailWithMsg("请求参数错误", c)
		return zero, false
	}
	return typed, true
}

// bindQuery 取中间件已经绑定好的查询参数。
func bindQuery[T any](c *gin.Context) (T, bool) {
	var zero T
	if c.IsAborted() {
		return zero, false
	}
	raw, ok := c.Get("requestQuery")
	if !ok {
		res.FailWithMsg("请求参数错误", c)
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		res.FailWithMsg("请求参数错误", c)
		return zero, false
	}
	return typed, true
}

// 编译期确认中间件的泛型实例可用。
var (
	_ = middleware.GetBindJson[IDListRequest]
	_ = middleware.GetBindQuery[GlobalListRequest]
)
