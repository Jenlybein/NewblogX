// Package http 是站点模块的 HTTP 适配器。
package http

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/site/app"
	"myblogx/internal/site/domain"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 聚合站点与轮播图的所有 HTTP 入口。
type Handler struct {
	svc *app.Service

	// sidebar 是右栏卡片服务；未装配时为 nil，相关接口回一条"未初始化"，
	// 而不是让零值 Handler（路由表测试用）panic。
	sidebar *app.SidebarService

	// onPublicSiteView 在公开配置（`GET /site/site`）被读取时调用一次，
	// 用来累计站点访问量（Redis 里的 total + 按天 hash）。
	//
	// 为什么放在 HTTP 层而不是用例层：这是"一次页面加载算一次访问"的埋点口径，
	// 属于接入层的事；用例层不需要知道流量是怎么计的。
	// 为 nil 时跳过（单测/路由表测试不关心计数）。
	onPublicSiteView func()
}

// Option 是 Handler 的可选依赖。
type Option func(*Handler)

// WithSidebar 注入右栏卡片服务。
func WithSidebar(sidebar *app.SidebarService) Option {
	return func(h *Handler) { h.sidebar = sidebar }
}

// NewHandler 构造处理器。onPublicSiteView 可为 nil。
func NewHandler(svc *app.Service, onPublicSiteView func(), opts ...Option) *Handler {
	if svc == nil {
		panic("site/http: 用例服务未注入")
	}
	h := &Handler{svc: svc, onPublicSiteView: onPublicSiteView}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// fail 交给统一响应层：apperr 自带中文文案。
func fail(c *gin.Context, err error) {
	if apperr.IsCode(err, apperr.CodeSiteConfigMissing) {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	res.FailWithError(err, c)
}

// ── 站点配置（公开读面） ───────────────────────────────────────

// QQLoginURL 返回 QQ 登录跳转地址。
// @Summary 获取 QQ 登录地址
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[string]
// @Router /site/qq_url [get]
func (h *Handler) QQLoginURL(c *gin.Context) {
	res.OkWithData(h.svc.QQLoginURL(), c)
}

// SEO 返回站点 SEO 信息。
// @Summary 获取站点 SEO 信息
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[SEOResponse]
// @Router /site/seo [get]
func (h *Handler) SEO(c *gin.Context) {
	seo := h.svc.PublicSEO()
	res.OkWithData(SEOResponse{
		ProjectTitle: seo.ProjectTitle,
		Icon:         seo.Icon,
		Keywords:     seo.Keywords,
		Description:  seo.Description,
	}, c)
}

// AIInfo 返回站点 AI 信息。
// @Summary 获取站点 AI 信息
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[AIResponse]
// @Router /site/ai_info [get]
func (h *Handler) AIInfo(c *gin.Context) {
	ai := h.svc.PublicAI()
	res.OkWithData(AIResponse{
		Enable:   ai.Enable,
		Nickname: ai.Nickname,
		Avatar:   ai.Avatar,
		Abstract: ai.Abstract,
	}, c)
}

// ── 轮播图 ─────────────────────────────────────────────────────

// BannerList 轮播图列表。
// @Summary 轮播图列表
// @Tags banner
// @Produce json
// @Param show query bool false "是否展示"
// @Success 200 {object} kernel.Envelope[kernel.HasMoreListData[BannerItem]]
// @Router /banners [get]
func (h *Handler) BannerList(c *gin.Context) {
	cr, ok := bindQuery[BannerListRequest](c)
	if !ok {
		return
	}
	if cr.Order != "" {
		// 旧实现没有开放排序白名单，传 order 会直接报错。
		res.FailWithMsg("排序字段错误", c)
		return
	}
	rows, hasMore, err := h.svc.ListBanners(c.Request.Context(), domain.BannerQuery{
		Page:  cr.Page,
		Limit: cr.Limit,
		Show:  cr.Show,
	})
	if err != nil {
		fail(c, err)
		return
	}
	list := make([]BannerItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, toBannerItem(row))
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// BannerCreate 创建轮播图（管理员）。
// @Summary 创建轮播图
// @Tags banner
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body BannerCreateRequest true "创建参数"
// @Success 200 {object} kernel.Envelope[BannerCreateResponse]
// @Router /banners [post]
func (h *Handler) BannerCreate(c *gin.Context) {
	cr, ok := bindJSON[BannerCreateRequest](c)
	if !ok {
		return
	}
	banner, err := h.svc.CreateBanner(c.Request.Context(), app.BannerInput{
		CoverImageID: cr.CoverImageID,
		Title:        cr.Title,
		Href:         cr.Href,
		Show:         cr.Show,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(BannerCreateResponse{
		ID:           banner.ID,
		CoverImageID: cr.CoverImageID,
		Cover:        banner.Cover,
		Href:         banner.Href,
		Title:        banner.Title,
		Show:         banner.Show,
	}, c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "banner_create",
		TargetType: "banner",
		TargetID:   strconv.FormatUint(banner.ID.Uint64(), 10),
		Success:    true,
		Message:    "创建轮播图成功",
	})
}

// BannerUpdate 更新轮播图（管理员）。
// @Summary 更新轮播图
// @Tags banner
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "轮播图 ID"
// @Param body body BannerCreateRequest true "更新参数"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /banners/{id} [put]
func (h *Handler) BannerUpdate(c *gin.Context) {
	id, ok := bindURI[IDRequest](c)
	if !ok {
		return
	}
	cr, ok := bindJSON[BannerCreateRequest](c)
	if !ok {
		return
	}
	err := h.svc.UpdateBanner(c.Request.Context(), id.ID.Uint64(), app.BannerInput{
		CoverImageID: cr.CoverImageID,
		Title:        cr.Title,
		Href:         cr.Href,
		Show:         cr.Show,
	})
	if err != nil {
		if apperr.IsCode(err, apperr.CodeBannerNotFound) {
			res.FailWithMsg("轮播图不存在", c)
			return
		}
		// 统一响应壳里 "校验失败"（业务码 1001）就是接口层的 400：
		// 请求被拒绝、什么都不落库，而不是被悄悄钳制。
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("更新轮播图成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "banner_update",
		TargetType: "banner",
		TargetID:   strconv.FormatUint(id.ID.Uint64(), 10),
		Success:    true,
		Message:    "更新轮播图成功",
	})
}

// BannerRemove 批量删除轮播图（管理员）。
// @Summary 批量删除轮播图
// @Tags banner
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body IDListRequest true "ID 列表"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /banners [delete]
func (h *Handler) BannerRemove(c *gin.Context) {
	cr, ok := bindJSON[IDListRequest](c)
	if !ok {
		return
	}
	count, err := h.svc.RemoveBanners(c.Request.Context(), toIDs(cr.IDList))
	if err != nil {
		fail(c, err)
		return
	}
	msg := fmt.Sprintf("请求删除轮播图%d个, 成功%d条", len(cr.IDList), count)
	res.OkWithMsg(msg, c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "banner_remove",
		TargetType: "banner",
		Success:    true,
		Message:    msg,
	})
}

// toBannerItem 把领域实体转成旧契约的 JSON 形状。
func toBannerItem(b domain.Banner) BannerItem {
	item := BannerItem{
		ID:        b.ID,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
		Show:      b.Show,
		Cover:     b.Cover,
		Href:      b.Href,
		Title:     b.Title,
	}
	if !b.CoverImageID.IsZero() {
		id := b.CoverImageID
		item.CoverImageID = &id
	}
	return item
}

// toIDs 把 ctype.ID 切片转成 uint64 切片。
func toIDs(ids []ctype.ID) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.Uint64())
	}
	return out
}
