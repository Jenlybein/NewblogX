package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/kernel"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
	"myblogx/middleware"
)

// 本文件是 HTTP 层的请求结构。
//
// 所有"可选字段"都用指针：PATCH 语义下，"没传"和"传空"是两件事，
// 用零值判断会把"清空简介"当成"不改简介"。

// UpdateRequest 是更新资料的请求体。
type UpdateRequest struct {
	Username *string `json:"username"`
	Nickname *string `json:"nickname"`
	Avatar   *string `json:"avatar"`
	Abstract *string `json:"abstract"`
}

// toDomain 转成领域层的更新对象。
func (r UpdateRequest) toDomain() domain.ProfileUpdate {
	return domain.ProfileUpdate{
		Username: r.Username,
		Nickname: r.Nickname,
		Avatar:   r.Avatar,
		Abstract: r.Abstract,
	}
}

// SettingsRequest 是更新偏好的请求体。
type SettingsRequest struct {
	LikeTagIDs               *[]uint64 `json:"like_tag_ids"`
	FavoritesVisibility      *bool     `json:"favorites_visibility"`
	FollowVisibility         *bool     `json:"followers_visibility"`
	FansVisibility           *bool     `json:"fans_visibility"`
	HomeStyleID              *uint64   `json:"home_style_id"`
	DiggNoticeEnabled        *bool     `json:"digg_notice_enabled"`
	CommentNoticeEnabled     *bool     `json:"comment_notice_enabled"`
	FavorNoticeEnabled       *bool     `json:"favor_notice_enabled"`
	PrivateChatNoticeEnabled *bool     `json:"private_chat_notice_enabled"`
	StrangerChatEnabled      *bool     `json:"stranger_msg_enabled"`
}

// toApp 转成用例层的补丁对象。
func (r SettingsRequest) toApp() app.SettingsPatch {
	return app.SettingsPatch{
		LikeTagIDs:               r.LikeTagIDs,
		FavoritesVisibility:      r.FavoritesVisibility,
		FollowVisibility:         r.FollowVisibility,
		FansVisibility:           r.FansVisibility,
		HomeStyleID:              r.HomeStyleID,
		DiggNoticeEnabled:        r.DiggNoticeEnabled,
		CommentNoticeEnabled:     r.CommentNoticeEnabled,
		FavorNoticeEnabled:       r.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: r.PrivateChatNoticeEnabled,
		StrangerChatEnabled:      r.StrangerChatEnabled,
	}
}

// ListRequest 是用户列表的查询参数。
type ListRequest struct {
	Page    int    `form:"page"`
	Size    int    `form:"size"`
	Keyword string `form:"keyword"`
	Status  int8   `form:"status"`
	Role    int8   `form:"role"`
}

// toDomain 转成领域查询。
func (r ListRequest) toDomain() domain.Query {
	q := domain.Query{
		Keyword: r.Keyword,
		Page:    r.Page,
		Size:    r.Size,
	}
	// 0 表示"不过滤"，不能当成某个具体状态。
	if r.Status != 0 {
		s := domain.Status(r.Status)
		q.Status = &s
	}
	if r.Role != 0 {
		role := domain.Role(r.Role)
		q.Role = &role
	}
	return q
}

// CreateRequest 是管理员创建用户的请求体。
type CreateRequest struct {
	Username string  `json:"username" binding:"required"`
	Nickname string  `json:"nickname" binding:"required"`
	Password string  `json:"password" binding:"required"`
	Email    *string `json:"email"`
	Role     int8    `json:"role"`
	Status   int8    `json:"status"`
	Addr     string  `json:"addr"`
}

// toApp 转成用例层输入。
func (r CreateRequest) toApp(ip string) app.CreateInput {
	return app.CreateInput{
		Username: r.Username,
		Nickname: r.Nickname,
		Password: r.Password,
		Email:    r.Email,
		Role:     domain.Role(r.Role),
		Status:   domain.Status(r.Status),
		IP:       ip,
		Addr:     r.Addr,
	}
}

// StatusRequest 是修改账号状态的请求体。
type StatusRequest struct {
	Status int8 `json:"status" binding:"required"`
}

// ------------------------------------------------------------------
// 绑定助手
// ------------------------------------------------------------------

// bindJSON 取中间件已经绑定好的请求体。
//
// 不能直接 c.MustGet：中间件在绑定失败时会 Abort 但不写入 context，
// 这里 MustGet 会 panic，把 400 变成 500。
func bindJSON[T any](c *gin.Context) (T, bool) {
	var zero T
	if c.IsAborted() {
		return zero, false
	}
	raw, ok := c.Get("requestJson")
	if !ok {
		failInvalid(c)
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		failInvalid(c)
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
		failInvalid(c)
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		failInvalid(c)
		return zero, false
	}
	return typed, true
}

// mustURI 取路径参数里的 ID。
func mustURI(c *gin.Context) *kernel.IDRequest {
	if c.IsAborted() {
		return nil
	}
	raw, ok := c.Get("requestUri")
	if !ok {
		failInvalid(c)
		return nil
	}
	typed, ok := raw.(kernel.IDRequest)
	if !ok || typed.ID == 0 {
		failInvalid(c)
		return nil
	}
	return &typed
}

// 编译期确认中间件的泛型实例可用（避免 import 只被文档引用）。
var _ = middleware.GetBindUri[kernel.IDRequest]
