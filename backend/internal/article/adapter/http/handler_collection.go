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
		ID:           ctype.ID(detail.Collection.ID()),
		Title:        detail.Collection.Title(),
		Description:  detail.Collection.Description(),
		Cover:        detail.Cover,
		CreatedAt:    detail.Collection.CreatedAt(),
		ArticleCount: detail.ArticleCount,
		Outline: OutlineTreeResponse{
			OwnerKind: 1,
			OwnerID:   ctype.ID(detail.Collection.ID()),
			Nodes:     toOutlineNodeResponses(detail.Tree),
		},
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
			ID:            ctype.ID(card.ID),
			Title:         card.Title,
			Description:   card.Description,
			Cover:         card.Cover,
			Displayed:     card.Displayed,
			ArticleCount:  card.ArticleCount,
			SortOrder:     card.SortOrder,
			FallbackCover: card.FallbackCover,
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

// UpdateCollection 改合集的标题 / 描述 / 封面（管理员）。
//
// 只改内容：展示开关与顺序各有独立接口，不会因为一次标题编辑被覆盖。
//
// @Summary 改合集
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionUpdateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id} [put]
func (h *Handler) UpdateCollection(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionUpdateRequest](c)

	if err := h.svc.UpdateCollection(c.Request.Context(), app.UpdateCollectionCmd{
		Actor:        h.viewerOf(c),
		CollectionID: collection.ID(uri.ID),
		Title:        cr.Title,
		Description:  cr.Description,
		Cover:        cr.Cover,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_update", uri.ID, true, "改合集")
	res.OkWithMsg("保存成功", c)
}

// DeleteCollection 软删一个合集（管理员）。
//
// 删合集不删文章：读者侧整块消失，后台列表也不再出现，文章本身照旧。
//
// @Summary 删合集
// @Tags collection
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/{id} [delete]
func (h *Handler) DeleteCollection(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	if err := h.svc.DeleteCollection(c.Request.Context(), app.DeleteCollectionCmd{
		Actor:        h.viewerOf(c),
		CollectionID: collection.ID(uri.ID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "collection_delete", uri.ID, true, "删合集")
	res.OkWithMsg("删除成功", c)
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
