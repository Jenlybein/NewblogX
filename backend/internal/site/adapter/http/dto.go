package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/site/domain"

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
	// Crop 是可选取景参数。可选是为了让老调用方（还没接取景的后台）照旧能用。
	//
	// 两种语义不同，别写混：
	//   · 创建：不传 = 缺省（居中、缩放 1）—— 新建必须有一条确定的取景；
	//   · 更新：不传 = **保留已存的取景** —— 否则改一个跳转链接就会静默抹掉运营调好的构图。
	Crop *BannerCrop `json:"crop"`
}

// BannerCrop 是取景参数的传输形状：焦点（图上 0–1 的相对坐标）+ 缩放（1–2）。
//
// 这里刻意不做范围校验：越界的拒绝发生在用例层（domain.Crop.Validate），
// 免得"能不能过"取决于请求从哪个入口进来。
type BannerCrop struct {
	FocusX float64 `json:"focus_x"`
	FocusY float64 `json:"focus_y"`
	Zoom   float64 `json:"zoom"`
}

// toDomain 把请求里的取景转成领域值；nil 表示调用方没传。
func (c *BannerCrop) toDomain() *domain.Crop {
	if c == nil {
		return nil
	}
	return &domain.Crop{FocusX: c.FocusX, FocusY: c.FocusY, Zoom: c.Zoom}
}

// fromDomainCrop 把领域取景转成传输形状。
func fromDomainCrop(c domain.Crop) BannerCrop {
	return BannerCrop{FocusX: c.FocusX, FocusY: c.FocusY, Zoom: c.Zoom}
}

// BannerListRequest 对应 GET /banners 的查询参数。
type BannerListRequest struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"`
	Show  *bool  `form:"show"`
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
	// Crop 总是有值：没有取景记录的历史图回传缺省（居中、缩放 1）。
	Crop BannerCrop `json:"crop"`
}

// BannerCreateResponse 是创建轮播图的返回。
type BannerCreateResponse struct {
	ID           ctype.ID   `json:"id"`
	CoverImageID ctype.ID   `json:"cover_image_id"`
	Cover        string     `json:"cover"`
	Href         string     `json:"href"`
	Show         bool       `json:"show"`
	Crop         BannerCrop `json:"crop"`
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

// ── 右栏卡片 ───────────────────────────────────────────────────

// SidebarCardItem 是读者侧一张右栏卡片的传输形状。
//
// 三种数据字段同时存在，前端按 kind 只读对应的那一个数组：
// 空数组即"这张卡没有数据"，读者站整块隐藏。这样前端不需要知道
// "某张卡的数据该从哪里取" —— 取数是后端的职责。
type SidebarCardItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	SortOrder int    `json:"sort_order"`
	// 热门文章卡的数据。
	HotArticles []SidebarArticleItem `json:"hot_articles"`
	// 最新评论卡的数据。
	LatestComments []SidebarCommentItem `json:"latest_comments"`
	// 标签云卡的数据（article_count 决定字号权重）。
	Tags []SidebarTagItem `json:"tags"`
}

// SidebarArticleItem 是热门文章卡的条目。
type SidebarArticleItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	DiggCount int    `json:"digg_count"`
	ViewCount int    `json:"view_count"`
}

// SidebarCommentItem 是最新评论卡的条目。
type SidebarCommentItem struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	ArticleID    string `json:"article_id"`
	ArticleTitle string `json:"article_title"`
	UserNickname string `json:"user_nickname"`
	CreatedAt    string `json:"created_at"`
}

// SidebarTagItem 是标签云卡的条目。
type SidebarTagItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ArticleCount int    `json:"article_count"`
}

// SidebarCardsResponse 是读者侧卡片列表的 data 载荷。
type SidebarCardsResponse struct {
	List []SidebarCardItem `json:"list"`
}

// AdminSidebarCardItem 是管理端一张卡片的传输形状。
type AdminSidebarCardItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	SortOrder int    `json:"sort_order"`
	Enabled   bool   `json:"enabled"`
}

// AdminSidebarCardsResponse 是管理端卡片列表的 data 载荷。
type AdminSidebarCardsResponse struct {
	List []AdminSidebarCardItem `json:"list"`
}

// SidebarCardSaveItem 是保存时提交的单张卡片。
//
// 不带 sort_order：顺序由 list 里的提交顺序决定，见 app.SidebarCardInput 的说明。
type SidebarCardSaveItem struct {
	Kind    string `json:"kind" binding:"required"`
	Title   string `json:"title"`
	Enabled bool   `json:"enabled"`
}

// SidebarCardsSaveRequest 是整表保存的请求体。
type SidebarCardsSaveRequest struct {
	List []SidebarCardSaveItem `json:"list" binding:"required"`
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
	_ = middleware.GetBindJson[SidebarCardsSaveRequest]
)
