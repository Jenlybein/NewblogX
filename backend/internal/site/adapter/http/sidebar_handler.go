package http

import (
	"time"

	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/site/app"
	"myblogx/internal/site/domain"

	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"

	"github.com/gin-gonic/gin"
)

// SidebarCards 返回读者侧右栏卡片（匿名可读）。
// @Summary 获取读者侧右栏卡片
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[SidebarCardsResponse]
// @Router /site/sidebar_cards [get]
func (h *Handler) SidebarCards(c *gin.Context) {
	if h.sidebar == nil {
		res.FailWithMsg("右栏卡片服务未初始化", c)
		return
	}
	views, err := h.sidebar.PublicCards(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	list := make([]SidebarCardItem, 0, len(views))
	for _, view := range views {
		list = append(list, toSidebarCardItem(view))
	}
	res.OkWithData(SidebarCardsResponse{List: list}, c)
}

// AdminSidebarCards 返回管理端右栏卡片（含停用，按运营顺序）。
// @Summary 获取右栏卡片配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[AdminSidebarCardsResponse]
// @Router /site/admin/sidebar_cards [get]
func (h *Handler) AdminSidebarCards(c *gin.Context) {
	if h.sidebar == nil {
		res.FailWithMsg("右栏卡片服务未初始化", c)
		return
	}
	rows, err := h.sidebar.AdminCards(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	list := make([]AdminSidebarCardItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, toAdminSidebarCardItem(row))
	}
	res.OkWithData(AdminSidebarCardsResponse{List: list}, c)
}

// UpdateSidebarCards 全量保存右栏卡片配置（管理员）。
// @Summary 保存右栏卡片配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SidebarCardsSaveRequest true "整表顺序即展示顺序"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/admin/sidebar_cards [put]
func (h *Handler) UpdateSidebarCards(c *gin.Context) {
	if h.sidebar == nil {
		res.FailWithMsg("右栏卡片服务未初始化", c)
		return
	}
	cr, ok := bindJSON[SidebarCardsSaveRequest](c)
	if !ok {
		return
	}
	inputs := make([]app.SidebarCardInput, 0, len(cr.List))
	for _, item := range cr.List {
		inputs = append(inputs, app.SidebarCardInput{
			Kind:    domain.SidebarCardKind(item.Kind),
			Title:   item.Title,
			Enabled: item.Enabled,
		})
	}
	if err := h.sidebar.ReplaceCards(c.Request.Context(), inputs); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("右栏卡片已保存", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "sidebar_cards_update",
		TargetType: "sidebar_card",
		Success:    true,
		Message:    "保存右栏卡片配置成功",
	})
}

// toSidebarCardItem 把读者侧视图转成传输形状。
func toSidebarCardItem(view app.SidebarCardView) SidebarCardItem {
	item := SidebarCardItem{
		ID:             view.ID.String(),
		Kind:           string(view.Kind),
		Title:          view.Title,
		SortOrder:      view.SortOrder,
		HotArticles:    make([]SidebarArticleItem, 0, len(view.HotArticles)),
		LatestComments: make([]SidebarCommentItem, 0, len(view.LatestComments)),
		Tags:           make([]SidebarTagItem, 0, len(view.Tags)),
	}
	for _, row := range view.HotArticles {
		item.HotArticles = append(item.HotArticles, SidebarArticleItem{
			ID:        row.ID.String(),
			Title:     row.Title,
			DiggCount: row.DiggCount,
			ViewCount: row.ViewCount,
		})
	}
	for _, row := range view.LatestComments {
		item.LatestComments = append(item.LatestComments, SidebarCommentItem{
			ID:           row.ID.String(),
			Content:      row.Content,
			ArticleID:    row.ArticleID.String(),
			ArticleTitle: row.ArticleTitle,
			UserNickname: row.UserNickname,
			CreatedAt:    formatSidebarTime(row.CreatedAt),
		})
	}
	for _, row := range view.Tags {
		item.Tags = append(item.Tags, SidebarTagItem{
			ID:           row.ID.String(),
			Title:        row.Title,
			ArticleCount: row.ArticleCount,
		})
	}
	return item
}

// toAdminSidebarCardItem 把领域实体转成管理端传输形状。
func toAdminSidebarCardItem(row domain.SidebarCard) AdminSidebarCardItem {
	return AdminSidebarCardItem{
		ID:        row.ID.String(),
		Kind:      string(row.Kind),
		Title:     row.Title,
		SortOrder: row.SortOrder,
		Enabled:   row.Enabled,
	}
}

// formatSidebarTime 把时间转成契约里的 RFC3339 字符串；零值返回空串。
func formatSidebarTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
