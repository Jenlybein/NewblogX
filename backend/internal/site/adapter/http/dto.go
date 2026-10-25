package http

import (
	"time"

	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

// ── 请求 ───────────────────────────────────────────────────────

// SiteInfoRequest 对应 /site/:name 里的名称。
type SiteInfoRequest struct {
	Name string `uri:"name"`
}

// IDRequest 对应 /banners/:id。
type IDRequest struct {
	ID ctype.ID `uri:"id"`
}

// IDListRequest 是批量删除的 ID 列表。
type IDListRequest struct {
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

// BannerCreateRequest 是创建/更新轮播图的入参。
type BannerCreateRequest struct {
	CoverImageID ctype.ID `json:"cover_image_id" binding:"required"`
	Href         string   `json:"href"`
	Show         bool     `json:"show"`
}

// BannerListRequest 对应 GET /banners 的查询参数。
type BannerListRequest struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"`
	Show  bool   `form:"show"`
}

// ── 响应 ───────────────────────────────────────────────────────

// BannerItem 是轮播图列表项。
//
// 旧接口直接返回 GORM 模型，字段顺序与 json tag 由 Model + BannerModel 决定，
// 这里逐字段对齐（含 id 序列化为字符串）。
type BannerItem struct {
	ID           ctype.ID   `json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
	Show         bool       `json:"show"`
	Cover        string     `json:"cover"`
	CoverImageID *ctype.ID  `json:"cover_image_id"`
	Href         string     `json:"href"`
}

// BannerCreateResponse 是创建轮播图的返回。
type BannerCreateResponse struct {
	ID           ctype.ID `json:"id"`
	CoverImageID ctype.ID `json:"cover_image_id"`
	Cover        string   `json:"cover"`
	Href         string   `json:"href"`
	Show         bool     `json:"show"`
}

// AIResponse 是公开的站点 AI 信息。
type AIResponse struct {
	Enable   bool   `json:"enable"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Abstract string `json:"abstract"`
}

// SEOResponse 是站点 SEO 摘要。
type SEOResponse struct {
	SiteTitle    string `json:"site_title"`
	ProjectTitle string `json:"project_title"`
	Logo         string `json:"logo"`
	Icon         string `json:"icon"`
	Keywords     string `json:"keywords"`
	Description  string `json:"description"`
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

// bindURI 取路径参数。
func bindURI[T any](c *gin.Context) (T, bool) {
	var zero T
	if c.IsAborted() {
		return zero, false
	}
	raw, ok := c.Get("requestUri")
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
	_ = middleware.GetBindQuery[BannerListRequest]
	_ = middleware.GetBindUri[SiteInfoRequest]
)
