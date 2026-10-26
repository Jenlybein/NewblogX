package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
)

// 本文件是兼容旧前端契约的请求/响应结构：字段名与旧 profile_api / user_man_api 一致，
// 路由路径不变，前端无需改动。

// UpdateInfoRequest 更新当前用户资料 + 偏好。
type UpdateInfoRequest struct {
	Username            *string     `json:"username"`
	Nickname            *string     `json:"nickname"`
	AvatarImageID       *ctype.ID   `json:"avatar_image_id"`
	Abstract            *string     `json:"abstract"`
	LikeTagIDs          *[]ctype.ID `json:"like_tag_ids"`
	FavoritesVisibility *bool       `json:"favorites_visibility"`
	FollowVisibility    *bool       `json:"followers_visibility"`
	FansVisibility      *bool       `json:"fans_visibility"`
	HomeStyleID         *ctype.ID   `json:"home_style_id"`
}

// toApp 转成用例输入。
func (r UpdateInfoRequest) toApp() app.UpdateInfoInput {
	return app.UpdateInfoInput{
		Profile: domain.ProfileUpdate{
			Username: r.Username,
			Nickname: r.Nickname,
			Abstract: r.Abstract,
		},
		AvatarImageID: r.AvatarImageID,
		Settings: app.SettingsPatch{
			LikeTagIDs:          r.LikeTagIDs,
			FavoritesVisibility: r.FavoritesVisibility,
			FollowVisibility:    r.FollowVisibility,
			FansVisibility:      r.FansVisibility,
			HomeStyleID:         r.HomeStyleID,
		},
	}
}

// AdminUpdateRequest 管理员更新指定用户。
type AdminUpdateRequest struct {
	UserID        ctype.ID  `json:"user_id" binding:"required"`
	Username      *string   `json:"username"`
	Nickname      *string   `json:"nickname"`
	AvatarImageID *ctype.ID `json:"avatar_image_id"`
	Abstract      *string   `json:"abstract"`
	Role          *int8     `json:"role"`
	Status        *int8     `json:"status"`
}

// AdminListRequest 管理端用户列表查询参数（沿用 page/limit/key）。
type AdminListRequest struct {
	Page  int    `form:"page"`
	Limit int    `form:"limit"`
	Key   string `form:"key"`
}

// AdminCreateRequest 管理员创建用户。
type AdminCreateRequest struct {
	Username string `json:"username" binding:"required,min=2,max=32"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"omitempty,min=2,max=32"`
	Email    string `json:"email" binding:"omitempty,email,max=256"`
}

// BaseInfoResponse 主页基础信息（旧契约字段名）。
type BaseInfoResponse struct {
	ID                  ctype.ID  `json:"id"`
	CodeAge             int       `json:"code_age"`
	AvatarImageID       *ctype.ID `json:"avatar_image_id,omitempty"`
	Avatar              string    `json:"avatar"`
	Nickname            string    `json:"nickname"`
	Abstract            string    `json:"abstract"`
	ViewCount           int       `json:"view_count"`
	FansCount           int       `json:"fans_count"`
	FollowCount         int       `json:"follow_count"`
	ArticleVisitedCount int       `json:"article_visited_count"`
	ArticleCount        int       `json:"article_count"`
	FavoritesVisibility bool      `json:"favorites_visibility"`
	FollowVisibility    bool      `json:"followers_visibility"`
	FansVisibility      bool      `json:"fans_visibility"`
	HomeStyleID         ctype.ID  `json:"home_style_id"`
	Relation            int8      `json:"relation"`
	Place               string    `json:"place"`
}

// AdminUserItemResponse 管理端用户列表的一行（旧契约字段名）。
type AdminUserItemResponse struct {
	ID          ctype.ID  `json:"id"`
	Nickname    string    `json:"nickname"`
	Avatar      string    `json:"avatar"`
	Username    string    `json:"username"`
	CreatedAt   time.Time `json:"created_at"`
	IP          string    `json:"ip"`
	Addr        string    `json:"addr"`
	LastLoginAt time.Time `json:"last_login_at"`
	// 角色与状态：管理端列表要显示「角色」列，编辑弹窗也要用它回填。
	// swaggertype 让契约印成 integer 而不是引用一个没有枚举的具名类型。
	Role   int8 `json:"role" swaggertype:"integer"`
	Status int8 `json:"status" swaggertype:"integer"`
}

// AdminCreateResponse 管理员创建用户的返回（旧契约字段名）。
type AdminCreateResponse struct {
	ID             ctype.ID `json:"id"`
	Username       string   `json:"username"`
	Nickname       string   `json:"nickname"`
	Email          *string  `json:"email"`
	Role           int8     `json:"role"`
	Status         int8     `json:"status"`
	RegisterSource int8     `json:"register_source"`
}
