package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
)

// 本文件是「热门置顶」运营位的 HTTP 适配层。
//
// 与 handler_sub.go 里的作者置顶（Tops / SetTop / RemoveTop，路径 /articles/top）
// 是**两组接口**，不要合并：作者置顶改的是个人主页，这里改的是首页运营位。
// 术语见 GLOSSARY「热门置顶 / 作者置顶」。

// HotTops 首页热门置顶（公开读）。
//
// 只返回**仍然公开**的文章；文章后来被隐藏/退回草稿/删除的置顶行留在库里，
// 首页这里直接查不到（过滤在仓储 SQL 里，不靠前端判断）。
//
// 「显示非管理员发布文章」关闭时，作者不是管理员的卡片同样查不到
// （ADR-0017 决策 4：热门置顶是读者端公开读面）。
//
// @Summary 热门置顶列表
// @Tags article
// @Produce json
// @Success 200 {object} kernel.Envelope[kernel.ListData[ArticleHotTopCardResponse]]
// @Router /articles/hot-top [get]
func (h *Handler) HotTops(c *gin.Context) {
	cards, err := h.svc.ListHotTopCards(c.Request.Context(), h.onlyAdminAuthors())
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]ArticleHotTopCardResponse, 0, len(cards))
	for _, card := range cards {
		out = append(out, ArticleHotTopCardResponse{
			ID:             ctype.ID(card.ArticleID),
			Title:          card.Title,
			Abstract:       card.Abstract,
			Cover:          card.Cover,
			ViewCount:      card.ViewCount,
			AuthorNickname: card.AuthorNickname,
			CategoryTitle:  card.CategoryTitle,
		})
	}
	res.OkWithList(out, len(out), c)
}

// AdminHotTops 热门置顶全量列表（管理员）。
//
// 与公开列表的差别只有一个：它包含**已失效**的置顶行（stale=true），
// 后台据此排序、标记与清理。
//
// @Summary 热门置顶全量列表（含失效）
// @Tags article
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[ArticleHotTopAdminItem]]
// @Router /articles/hot-top/admin [get]
func (h *Handler) AdminHotTops(c *gin.Context) {
	items, err := h.svc.ListHotTopItems(c.Request.Context(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]ArticleHotTopAdminItem, 0, len(items))
	for _, item := range items {
		out = append(out, ArticleHotTopAdminItem{
			ArticleID: ctype.ID(item.ArticleID),
			SortOrder: item.SortOrder,
			Stale:     item.Stale(),
			Title:     item.Title,
			Cover:     item.Cover,
		})
	}
	res.OkWithList(out, len(out), c)
}

// SetHotTop 设置热门置顶（幂等）。
//
// 重复设置不报错、不改顺序；超过 12 条返回可读错误。
//
// @Summary 设置热门置顶
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/{id}/admin/hot-top [post]
func (h *Handler) SetHotTop(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.toggleHotTop(c, uri.ID, false)
}

// RemoveHotTop 取消热门置顶（幂等）。
//
// 不要求文章仍然存在或仍然公开：失效行必须能被清理。
//
// @Summary 取消热门置顶
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/{id}/admin/hot-top [delete]
func (h *Handler) RemoveHotTop(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.toggleHotTop(c, uri.ID, true)
}

// toggleHotTop 是设置/取消的公共逻辑。
func (h *Handler) toggleHotTop(c *gin.Context, id ctype.ID, remove bool) {
	articleID := domain.ArticleID(id)

	if err := h.svc.SetHotTop(c.Request.Context(), app.HotTopCmd{
		Actor:     h.viewerOf(c),
		ArticleID: articleID,
		Remove:    remove,
	}); err != nil {
		h.fail(c, err)
		return
	}

	if remove {
		h.audit(c, "article_hot_top_remove", articleID, true, "取消热门置顶")
		res.OkWithMsg("取消成功", c)
		return
	}
	h.audit(c, "article_hot_top_set", articleID, true, "设置热门置顶")
	res.OkWithMsg("设置成功", c)
}

// ReplaceHotTopOrder 全量顺序替换热门置顶（管理员）。
//
// 请求体是**完整且有序**的置顶文章 ID 列表；集合与当前不一致时整单拒绝
// （避免过期页面点一次"上移"就把别人刚加的一条置顶删掉）。
//
// @Summary 热门置顶排序
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ArticleHotTopOrderRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/hot-top/order [patch]
func (h *Handler) ReplaceHotTopOrder(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleHotTopOrderRequest](c)

	ids := make([]domain.ArticleID, 0, len(cr.ArticleIDs))
	for _, id := range cr.ArticleIDs {
		ids = append(ids, domain.ArticleID(id))
	}
	if err := h.svc.ReplaceHotTopOrder(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "article_hot_top_order", "", true, "热门置顶排序")
	res.OkWithMsg("排序成功", c)
}
