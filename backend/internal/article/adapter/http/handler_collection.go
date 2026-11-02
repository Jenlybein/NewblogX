package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
)

// 本文件是「合集」的 HTTP 适配层。
//
// 读者侧两个接口（列表 / 详情）对匿名开放，但受「展示」开关与
// 「显示非管理员发布文章」开关共同约束；写接口全部在管理员分组下。
// 合集是管理员专属的运营产物，作者不参与创建（见 GLOSSARY「合集」）。

// Collections 合集列表（公开读，分页）。
//
// 只返回**展示中**的合集；未展示的合集留在表里（后台仍可编辑），这里查不到。
//
// @Summary 合集列表
// @Tags collection
// @Produce json
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} kernel.Envelope[kernel.ListData[CollectionCardResponse]]
// @Router /collections [get]
func (h *Handler) Collections(c *gin.Context) {
	cr := middleware.GetBindQuery[CollectionListRequest](c)

	cards, total, err := h.svc.ListCollectionCards(c.Request.Context(),
		h.onlyAdminAuthors(), domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]CollectionCardResponse, 0, len(cards))
	for _, card := range cards {
		out = append(out, CollectionCardResponse{
			ID:           ctype.ID(card.ID),
			Title:        card.Title,
			Description:  card.Description,
			Cover:        card.Cover,
			ArticleCount: card.ArticleCount,
			CreatedAt:    card.CreatedAt,
		})
	}
	res.OkWithList(out, int(total), c)
}

// CollectionDetail 合集详情（公开读）。
//
// 未展示的合集对读者按**不存在**处理（不返回"这个合集未发布"——那能用来探测
// 还没公布的专题）；管理员仍能打开，后台编辑要用。
//
// @Summary 合集详情
// @Tags collection
// @Produce json
// @Param id path string true "合集 ID"
// @Success 200 {object} kernel.Envelope[CollectionDetailResponse]
// @Router /collections/{id} [get]
func (h *Handler) CollectionDetail(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)

	detail, err := h.svc.CollectionDetail(c.Request.Context(), collection.ID(cr.ID),
		h.viewerOf(c), h.onlyAdminAuthors())
	if err != nil {
		h.fail(c, err)
		return
	}

	out := CollectionDetailResponse{
		ID:          ctype.ID(detail.Collection.ID()),
		Title:       detail.Collection.Title(),
		Description: detail.Collection.Description(),
		Cover:       detail.Collection.Cover(),
		CreatedAt:   detail.Collection.CreatedAt(),
		Articles:    make([]CollectionArticleItemResponse, 0, len(detail.Articles)),
	}
	for _, item := range detail.Articles {
		out.Articles = append(out.Articles, CollectionArticleItemResponse{
			ArticleID:      ctype.ID(item.ArticleID),
			SortOrder:      item.SortOrder,
			Title:          item.Title,
			Abstract:       item.Abstract,
			Cover:          item.Cover,
			ViewCount:      item.ViewCount,
			AuthorNickname: item.AuthorNickname,
			CategoryTitle:  item.CategoryTitle,
		})
	}
	res.OkWithData(out, c)
}

// AdminCollections 后台合集列表（含未展示的合集）。
//
// 与公开列表的差别：未展示的合集也出现，且带 displayed 与 sort_order，
// 后台据此开关展示与调整合集顺序。
//
// @Summary 后台合集列表（含未展示）
// @Tags collection
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[CollectionAdminCard]]
// @Router /collections/admin [get]
func (h *Handler) AdminCollections(c *gin.Context) {
	cards, err := h.svc.ListAdminCollectionCards(c.Request.Context(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]CollectionAdminCard, 0, len(cards))
	for _, card := range cards {
		out = append(out, CollectionAdminCard{
			ID:           ctype.ID(card.ID),
			Title:        card.Title,
			Description:  card.Description,
			Cover:        card.Cover,
			Displayed:    card.Displayed,
			ArticleCount: card.ArticleCount,
			SortOrder:    card.SortOrder,
		})
	}
	res.OkWithList(out, len(out), c)
}

// CreateCollection 新建合集（管理员）。
//
// @Summary 新建合集
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CollectionCreateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[CollectionCreatedResponse]
// @Router /collections [post]
func (h *Handler) CreateCollection(c *gin.Context) {
	cr := middleware.GetBindJson[CollectionCreateRequest](c)

	id, err := h.svc.CreateCollection(c.Request.Context(), app.CreateCollectionCmd{
		Actor:       h.viewerOf(c),
		Title:       cr.Title,
		Description: cr.Description,
		Cover:       cr.Cover,
		Displayed:   cr.Displayed,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_create", ctype.ID(id), true, "新建合集")
	res.OkWithData(CollectionCreatedResponse{ID: ctype.ID(id)}, c)
}

// SetCollectionDisplay 切换合集的展示开关（管理员）。
//
// @Summary 合集展示开关
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionDisplayRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id}/display [patch]
func (h *Handler) SetCollectionDisplay(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionDisplayRequest](c)

	if err := h.svc.SetCollectionDisplay(c.Request.Context(), app.SetCollectionDisplayCmd{
		Actor:        h.viewerOf(c),
		CollectionID: collection.ID(uri.ID),
		Displayed:    cr.Displayed,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_display", uri.ID, true, "切换合集展示")
	res.OkWithMsg("设置成功", c)
}

// IncludeCollectionArticle 把一篇文章收录进合集（管理员）。
//
// @Summary 收录文章进合集
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionArticleRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id}/articles [post]
func (h *Handler) IncludeCollectionArticle(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionArticleRequest](c)

	if err := h.svc.IncludeArticle(c.Request.Context(), app.IncludeArticleCmd{
		Actor:        h.viewerOf(c),
		CollectionID: collection.ID(uri.ID),
		ArticleID:    domain.ArticleID(cr.ArticleID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_include", cr.ArticleID, true, "收录文章进合集")
	res.OkWithMsg("收录成功", c)
}

// RemoveCollectionArticle 从合集中移除一篇文章（管理员）。
//
// 幂等：不在合集里也返回成功（后台批量清理时不该因为一行过期数据整单失败）。
//
// @Summary 从合集移除文章
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionArticleRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id}/articles [delete]
func (h *Handler) RemoveCollectionArticle(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionArticleRequest](c)

	if err := h.svc.RemoveArticle(c.Request.Context(), app.RemoveArticleCmd{
		Actor:        h.viewerOf(c),
		CollectionID: collection.ID(uri.ID),
		ArticleID:    domain.ArticleID(cr.ArticleID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_remove", cr.ArticleID, true, "从合集移除文章")
	res.OkWithMsg("移除成功", c)
}

// ReplaceCollectionOrder 合集内的全量顺序替换（管理员）。
//
// 请求体是**完整且有序**的文章 ID 列表；集合与当前收录不一致时整单拒绝
// （避免过期页面点一次"上移"就把别人刚收录的一篇静默删掉）。
//
// @Summary 合集内文章排序
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionArticleOrderRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id}/articles/order [patch]
func (h *Handler) ReplaceCollectionOrder(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionArticleOrderRequest](c)

	ids := make([]collection.ArticleID, 0, len(cr.ArticleIDs))
	for _, id := range cr.ArticleIDs {
		ids = append(ids, collection.ArticleID(id))
	}
	if err := h.svc.ReplaceCollectionArticleOrder(c.Request.Context(), h.viewerOf(c),
		collection.ID(uri.ID), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_order", uri.ID, true, "合集内排序")
	res.OkWithMsg("排序成功", c)
}
