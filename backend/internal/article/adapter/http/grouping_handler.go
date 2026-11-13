package http

import (
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
)

// 本文件是合集 / 项目分组的 HTTP 入口（ADR-0019）。
//
// 两个实体共用同一组 handler，归属由路径段（collections / projects）决定：
// 语义完全一致，写两份 handler 只会让"删分组时忘记把条目退回未分组"这类
// 差异只修在一边。
//
// 路由形状：
//
//	GET    /<kind>/groups          后台分组读面（分组 + 组内顺序 + 未分组）
//	PUT    /<kind>/groups          新建或改名一个分组（立即生效）
//	DELETE /<kind>/groups/:id      删除分组（条目退回未分组，不删条目）
//	PUT    /<kind>/groups/order    整树顺序整份覆盖（幂等）
//	PUT    /<kind>/:id/compose     字段 + 目录树一次原子提交

// groupKind 取本端点所属的实体。
//
// 由路由注册时绑进上下文（见 router.go 的 bindKind）：**不靠路径字符串猜**——
// 用 `strings.Contains(path, "/collections/")` 决定写哪张表，等于让 URL 形状
// 变成数据路由的一部分，改一次路径就静默写错表。
func groupKind(c *gin.Context) (outline.OwnerKind, bool) {
	raw, ok := c.Get(kindContextKey)
	if !ok {
		return 0, false
	}
	kind, ok := raw.(outline.OwnerKind)
	return kind, ok
}

// kindContextKey 是"本端点属于哪个实体"在 gin 上下文里的键。
const kindContextKey = "article.grouping.kind"

// BindKind 返回一个把实体归属绑进上下文、再交给 handler 的中间件。
//
// 路由注册时一个端点绑一次，handler 里不再关心自己挂在哪个前缀下。
func BindKind(kind outline.OwnerKind, h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(kindContextKey, kind)
		h(c)
	}
}

// GroupTreeView 后台分组读面。
// @Summary 分组读面
// @Tags collection
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[GroupTreeResponse]
// @Router /collections/groups [get]
// @Router /projects/groups [get]
func (h *Handler) GroupTreeView(c *gin.Context) {
	kind, ok := groupKind(c)
	if !ok {
		res.FailWithMsg("未知的实体", c)
		return
	}
	tree, err := h.svc.LoadGroupTree(c.Request.Context(), h.viewerOf(c), kind)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(toGroupTreeResponse(tree), c)
}

// GroupSaveView 新建或改名一个分组（管理员，立即生效）。
//
// 分组 CRUD 不参与拖拽的 10 秒防抖：用户点了"删除分组"却要等 10 秒才生效是
// 不可接受的，所以建 / 改名 / 删各是一次独立提交。
// @Summary 新建或改名分组
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body GroupSaveRequest true "分组标题（id 为空表示新建）"
// @Success 200 {object} kernel.Envelope[GroupNodeResponse]
// @Router /collections/groups [put]
// @Router /projects/groups [put]
func (h *Handler) GroupSaveView(c *gin.Context) {
	kind, ok := groupKind(c)
	if !ok {
		res.FailWithMsg("未知的实体", c)
		return
	}
	cr := middleware.GetBindJson[GroupSaveRequest](c)
	saved, err := h.svc.SaveGroup(c.Request.Context(), app.SaveGroupCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: kind,
		ID:        grouping.ID(cr.ID),
		Title:     cr.Title,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(GroupNodeResponse{ID: ctype.ID(saved.ID()), Title: saved.Title(), Items: []GroupItemResponse{}}, c)
}

// GroupDeleteView 删除一个分组（管理员，立即生效）。
//
// 只删编排：分组下的条目退回未分组，合集 / 项目本身一行不动。
// @Summary 删除分组
// @Tags collection
// @Produce json
// @Security BearerAuth
// @Param id path string true "分组 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/groups/{id} [delete]
// @Router /projects/groups/{id} [delete]
func (h *Handler) GroupDeleteView(c *gin.Context) {
	kind, ok := groupKind(c)
	if !ok {
		res.FailWithMsg("未知的实体", c)
		return
	}
	cr := middleware.GetBindUri[kernel.IDRequest](c)
	if err := h.svc.DeleteGroup(c.Request.Context(), app.DeleteGroupCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: kind,
		ID:        grouping.ID(cr.ID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithMsg("删除成功", c)
}

// GroupOrderView 整份覆盖分组顺序与条目归属（管理员，幂等）。
// @Summary 分组整树换序
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body GroupOrderRequest true "整树顺序"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /collections/groups/order [put]
// @Router /projects/groups/order [put]
func (h *Handler) GroupOrderView(c *gin.Context) {
	kind, ok := groupKind(c)
	if !ok {
		res.FailWithMsg("未知的实体", c)
		return
	}
	cr := middleware.GetBindJson[GroupOrderRequest](c)

	tree := grouping.TreeOrder{
		Groups:    make([]grouping.GroupOrder, 0, len(cr.Groups)),
		Ungrouped: cr.Ungrouped,
	}
	for _, g := range cr.Groups {
		tree.Groups = append(tree.Groups, grouping.GroupOrder{ID: grouping.ID(g.ID), Items: g.Items})
	}
	if err := h.svc.ReplaceGroupOrder(c.Request.Context(), app.ReplaceGroupOrderCmd{
		Actor:     h.viewerOf(c),
		OwnerKind: kind,
		Tree:      tree,
	}); err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithMsg("保存成功", c)
}

// CollectionComposeView 合集字段与目录树的一次原子提交（管理员）。
// @Summary 合集字段 + 目录树一次提交
// @Tags collection
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "合集 ID"
// @Param body body CollectionComposeRequest true "字段与整棵树"
// @Success 200 {object} kernel.Envelope[CollectionOutlineTreeResponse]
// @Router /collections/{id}/compose [put]
func (h *Handler) CollectionComposeView(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)
	body := middleware.GetBindJson[CollectionComposeRequest](c)

	detail, err := h.svc.ComposeCollection(c.Request.Context(), app.ComposeCollectionCmd{
		Actor:       h.viewerOf(c),
		ID:          collection.ID(cr.ID),
		Title:       body.Title,
		Description: body.Description,
		Cover:       body.Cover,
		Nodes:       toNamedOutlineInputs(body.Nodes),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	var out CollectionOutlineTreeResponse
	fillNamedCollectionOutlineSave(&out, toOutlineTreeResponse(detail))
	res.OkWithData(out, c)
}

// ProjectComposeView 项目字段与目录树的一次原子提交（管理员）。
// @Summary 项目字段 + 目录树一次提交
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectComposeRequest true "字段与整棵树"
// @Success 200 {object} kernel.Envelope[ProjectOutlineTreeResponse]
// @Router /projects/{id}/compose [put]
func (h *Handler) ProjectComposeView(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)
	body := middleware.GetBindJson[ProjectComposeRequest](c)

	detail, err := h.svc.ComposeProject(c.Request.Context(), app.ComposeProjectCmd{
		Actor:       h.viewerOf(c),
		ID:          project.ID(cr.ID),
		Title:       body.Title,
		Description: body.Description,
		Cover:       body.Cover,
		Nodes:       toNamedOutlineInputs(body.Nodes),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	var out ProjectOutlineTreeResponse
	fillNamedProjectOutlineSave(&out, toOutlineTreeResponse(detail))
	res.OkWithData(out, c)
}

// PublicGroupTreeView 读者侧分组读面（票 353）。
//
// 只列展示中的实体，且**不返回「未分组」标题**：没有分组的实体作为所有分组
// 之后的尾块平铺（`ungrouped` 里只有 id，读者侧不渲染标题）。
// @Summary 读者侧分组读面
// @Tags collection
// @Produce json
// @Success 200 {object} kernel.Envelope[GroupTreeResponse]
// @Router /collections/grouped [get]
// @Router /projects/grouped [get]
func (h *Handler) PublicGroupTreeView(c *gin.Context) {
	kind, ok := groupKind(c)
	if !ok {
		res.FailWithMsg("未知的实体", c)
		return
	}
	tree, err := h.svc.PublicGroupTree(c.Request.Context(), kind)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(toGroupTreeResponse(tree), c)
}
