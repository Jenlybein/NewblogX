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

// 本文件是「目录树」的 HTTP 适配层。
//
// 后台树编辑器的最小闭环只有两个动作：读回整棵树、整棵树一次性提交。
// 新建文件夹 / 批量收录 / 上移 / 下移 / 缩进 / 移出都在页面上完成，保存时
// 提交完整树——服务端不做"单步移动"接口，避免出现第二套顺序真相（ADR-0023）。

// AdminCollectionOutline 读回某个合集的目录树（管理员）。
//
// @Summary 后台合集目录树
// @Tags outline
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Success 200 {object} kernel.Envelope[OutlineTreeResponse]
// @Router /collections/{id}/outline/admin [get]
func (h *Handler) AdminCollectionOutline(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.adminOutline(c, outline.OwnerCollection, outline.ID(uri.ID))
}

// SaveCollectionOutline 整棵树一次性提交（管理员）。
//
// @Summary 保存合集目录树
// @Tags outline
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body OutlineSaveRequest true "请求体"
// @Success 200 {object} kernel.Envelope[OutlineTreeResponse]
// @Router /collections/{id}/outline [put]
func (h *Handler) SaveCollectionOutline(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.saveOutline(c, outline.OwnerCollection, outline.ID(uri.ID))
}

// AdminProjectOutline 读回某个项目的目录树（管理员）。
//
// @Summary 后台项目目录树
// @Tags outline
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[OutlineTreeResponse]
// @Router /projects/{id}/outline/admin [get]
func (h *Handler) AdminProjectOutline(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.adminOutline(c, outline.OwnerProject, outline.ID(uri.ID))
}

// SaveProjectOutline 整棵树一次性提交（管理员）。
//
// @Summary 保存项目目录树
// @Tags outline
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body OutlineSaveRequest true "请求体"
// @Success 200 {object} kernel.Envelope[OutlineTreeResponse]
// @Router /projects/{id}/outline [put]
func (h *Handler) SaveProjectOutline(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	h.saveOutline(c, outline.OwnerProject, outline.ID(uri.ID))
}

// adminOutline 是两个实体共用的读实现。
func (h *Handler) adminOutline(c *gin.Context, kind outline.OwnerKind, ownerID outline.ID) {
	detail, err := h.svc.AdminOutline(c.Request.Context(), h.viewerOf(c), kind, ownerID)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(toOutlineTreeResponse(detail), c)
}

// saveOutline 是两个实体共用的写实现。
func (h *Handler) saveOutline(c *gin.Context, kind outline.OwnerKind, ownerID outline.ID) {
	cr := middleware.GetBindJson[OutlineSaveRequest](c)

	nodes := make([]app.OutlineInputNode, 0, len(cr.Nodes))
	for _, node := range cr.Nodes {
		nodes = append(nodes, toOutlineInput(node))
	}

	detail, err := h.svc.SaveOutline(c.Request.Context(), app.SaveOutlineCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: kind,
		OwnerID:   ownerID,
		Nodes:     nodes,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	emitOutlineAudit(c, kind, ownerID)
	res.OkWithData(toOutlineTreeResponse(detail), c)
}

// emitOutlineAudit 记一条后台变更审计。
//
// 不复用 h.audit：那个 helper 把 target_type 写死成 "article"，树属于合集 / 项目。
func emitOutlineAudit(c *gin.Context, kind outline.OwnerKind, ownerID outline.ID) {
	targetType := "collection"
	if kind == outline.OwnerProject {
		targetType = "project"
	}
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "outline_save",
		TargetType: targetType,
		TargetID:   ownerID.String(),
		Success:    true,
		Message:    "保存目录树",
	})
}

// toOutlineInput 把请求 DTO 转成用例输入（递归保持父子关系）。
func toOutlineInput(node OutlineNodeRequest) app.OutlineInputNode {
	children := make([]app.OutlineInputNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, toOutlineInput(child))
	}
	return app.OutlineInputNode{
		ID:        outline.ID(node.ID),
		Kind:      outline.NodeKind(node.Kind),
		Title:     node.Title,
		ArticleID: outline.ArticleID(node.ArticleID),
		Children:  children,
	}
}

// toOutlineTreeResponse 把读模型转成嵌套响应。
func toOutlineTreeResponse(detail *outline.Detail) OutlineTreeResponse {
	out := OutlineTreeResponse{
		OwnerKind: int(detail.OwnerKind),
		OwnerID:   ctype.ID(detail.OwnerID),
		Nodes:     toOutlineNodeResponses(detail.Tree),
	}
	return out
}

func toOutlineNodeResponses(items []outline.ViewNode) []OutlineNodeResponse {
	out := make([]OutlineNodeResponse, 0, len(items))
	for _, item := range items {
		row := OutlineNodeResponse{
			ID:           ctype.ID(item.Node.ID()),
			ParentID:     ctype.ID(item.Node.ParentID()),
			Kind:         int(item.Node.Kind()),
			Title:        item.Node.Title(),
			ArticleID:    ctype.ID(item.Node.ArticleID()),
			SortOrder:    item.Node.SortOrder(),
			ArticleTitle: item.ArticleTitle,
			ArticleCover: item.ArticleCover,
			Stale:        item.Stale(),
			StaleReason:  string(item.StaleReason()),
			Children:     toOutlineNodeResponses(item.Children),
		}
		out = append(out, row)
	}
	return out
}
