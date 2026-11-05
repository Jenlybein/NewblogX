package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
)

// 本文件是 #274 expand 期两个具名端点的 HTTP 适配层。
//
// 合集：`GET|PUT /collections/{id}/outline-tree`
// 项目：`GET|PUT /projects/{id}/outline-tree`
// 两个 GET 是公开读（阅读页左栏 / 详情页按 `?from=` 找回同一棵树），两个 PUT
// 在管理员分组下（整树提交）。端点身份决定走哪张具名表，旧共享端点票内暂留。

// CollectionOutlineTree 读合集具名树（公开读）。
//
// `from` 是阅读页透传的 `<kind>:<id>` 来路；`article_id` 是阅读页当前文章，
// 带它才回答上一章 / 下一章。
//
// @Summary 合集具名目录树
// @Tags collection
// @Produce json
// @Param id path string true "合集 ID"
// @Param from query string false "来源（collection:<id>）"
// @Param article_id query string false "阅读页当前文章 ID"
// @Success 200 {object} kernel.Envelope[CollectionOutlineTreeResponse]
// @Router /collections/{id}/outline-tree [get]
func (h *Handler) CollectionOutlineTree(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	q := middleware.GetBindQuery[NamedOutlineTreeRequest](c)
	view, err := h.svc.NamedOutlineTree(c.Request.Context(), outline.OwnerCollection,
		outline.ID(uri.ID), q.From, ctype.ID(q.ArticleID), h.onlyAdminAuthors(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	var out CollectionOutlineTreeResponse
	fillNamedCollectionOutline(&out, view)
	res.OkWithData(out, c)
}

// SaveCollectionOutlineTree 整树提交合集具名树（管理员）。
//
// @Summary 保存合集具名目录树
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionOutlineSaveRequest true "请求体"
// @Success 200 {object} kernel.Envelope[CollectionOutlineTreeResponse]
// @Router /collections/{id}/outline-tree [put]
func (h *Handler) SaveCollectionOutlineTree(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[CollectionOutlineSaveRequest](c)
	detail, err := h.svc.SaveNamedOutline(c.Request.Context(), app.SaveNamedOutlineCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   outline.ID(uri.ID),
		Nodes:     toNamedOutlineInputs(cr.Nodes),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	emitOutlineAudit(c, outline.OwnerCollection, outline.ID(uri.ID))
	var out CollectionOutlineTreeResponse
	fillNamedCollectionOutlineSave(&out, toOutlineTreeResponse(detail))
	res.OkWithData(out, c)
}

// ProjectOutlineTree 读项目具名树（公开读）。
//
// @Summary 项目具名目录树
// @Tags project
// @Produce json
// @Param id path string true "项目 ID"
// @Param from query string false "来源（project:<id>）"
// @Param article_id query string false "阅读页当前文章 ID"
// @Success 200 {object} kernel.Envelope[ProjectOutlineTreeResponse]
// @Router /projects/{id}/outline-tree [get]
func (h *Handler) ProjectOutlineTree(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	q := middleware.GetBindQuery[NamedOutlineTreeRequest](c)
	view, err := h.svc.NamedOutlineTree(c.Request.Context(), outline.OwnerProject,
		outline.ID(uri.ID), q.From, ctype.ID(q.ArticleID), h.onlyAdminAuthors(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	var out ProjectOutlineTreeResponse
	fillNamedProjectOutline(&out, view)
	res.OkWithData(out, c)
}

// SaveProjectOutlineTree 整树提交项目具名树（管理员）。
//
// @Summary 保存项目具名目录树
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectOutlineSaveRequest true "请求体"
// @Success 200 {object} kernel.Envelope[ProjectOutlineTreeResponse]
// @Router /projects/{id}/outline-tree [put]
func (h *Handler) SaveProjectOutlineTree(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectOutlineSaveRequest](c)
	detail, err := h.svc.SaveNamedOutline(c.Request.Context(), app.SaveNamedOutlineCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: outline.OwnerProject,
		OwnerID:   outline.ID(uri.ID),
		Nodes:     toNamedOutlineInputs(cr.Nodes),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	emitOutlineAudit(c, outline.OwnerProject, outline.ID(uri.ID))
	var out ProjectOutlineTreeResponse
	fillNamedProjectOutlineSave(&out, toOutlineTreeResponse(detail))
	res.OkWithData(out, c)
}

// toNamedOutlineInputs 把请求 DTO 转成用例输入（递归保持父子关系）。
func toNamedOutlineInputs(nodes []OutlineNodeRequest) []app.OutlineInputNode {
	out := make([]app.OutlineInputNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, toOutlineInput(node))
	}
	return out
}

// fillNamedCollectionOutline 把读者读模型填进合集具名响应。
func fillNamedCollectionOutline(out *CollectionOutlineTreeResponse, view *app.NamedOutlineView) {
	out.HasTree = view.HasTree
	out.OwnerKind = int(view.OwnerKind)
	out.OwnerID = ctype.ID(view.OwnerID)
	out.Nodes = toOutlineNodeResponses(view.Tree)
	out.ArticleCount = view.ArticleCount
	out.Cover = view.Cover
	out.Current = toOutlineArticleRefResponse(view.Current)
	out.Previous = toOutlineArticleRefResponse(view.Previous)
	out.Next = toOutlineArticleRefResponse(view.Next)
}

// fillNamedProjectOutline 把读者读模型填进项目具名响应。
func fillNamedProjectOutline(out *ProjectOutlineTreeResponse, view *app.NamedOutlineView) {
	out.HasTree = view.HasTree
	out.OwnerKind = int(view.OwnerKind)
	out.OwnerID = ctype.ID(view.OwnerID)
	out.Nodes = toOutlineNodeResponses(view.Tree)
	out.ArticleCount = view.ArticleCount
	out.Cover = view.Cover
	out.Current = toOutlineArticleRefResponse(view.Current)
	out.Previous = toOutlineArticleRefResponse(view.Previous)
	out.Next = toOutlineArticleRefResponse(view.Next)
}

// fillNamedCollectionOutlineSave 把后台读面填进合集具名响应。
// 整树提交没有篇数 / 封面 / 相邻文章：那些只对读者读面有意义。
func fillNamedCollectionOutlineSave(out *CollectionOutlineTreeResponse, tree OutlineTreeResponse) {
	out.OwnerKind = tree.OwnerKind
	out.OwnerID = tree.OwnerID
	out.Nodes = tree.Nodes
}

// fillNamedProjectOutlineSave 把后台读面填进项目具名响应。
func fillNamedProjectOutlineSave(out *ProjectOutlineTreeResponse, tree OutlineTreeResponse) {
	out.OwnerKind = tree.OwnerKind
	out.OwnerID = tree.OwnerID
	out.Nodes = tree.Nodes
}
