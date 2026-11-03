package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
)

// 本文件是「项目」的 HTTP 适配层（#193 读者侧公开读 + #197 管理端）。
//
// 项目与合集并列、页面形态复用，但**数据与表完全独立**；管理端增删改与编排
// 全部挂在独立的管理员分组下（见 router.go 的 projectAdminGroup）。
//
// 读者侧两个接口对匿名开放，但受「展示」开关与「显示非管理员发布文章」
// 开关共同约束：只出展示中的项目，未展示的项目按不存在处理。

// Projects 项目列表（公开读，分页）。
//
// 只返回**展示中**的项目；未展示的项目留在表里（后台 #197 仍可编辑），这里查不到。
// 篇数只算仍公开的文章，封面为空时回退首篇公开文章的封面（都在读模型里算好）。
//
// @Summary 项目列表
// @Tags project
// @Produce json
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} kernel.Envelope[kernel.ListData[ProjectCardResponse]]
// @Router /projects [get]
func (h *Handler) Projects(c *gin.Context) {
	cr := middleware.GetBindQuery[ProjectListRequest](c)

	cards, total, err := h.svc.ListProjectCards(c.Request.Context(),
		h.onlyAdminAuthors(), domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]ProjectCardResponse, 0, len(cards))
	for _, card := range cards {
		out = append(out, ProjectCardResponse{
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

// ProjectDetail 项目详情（公开读）。
//
// 未展示的项目对读者按**不存在**处理（不返回"这个项目未发布"——那能用来探测
// 还没公布的编排）；管理端读面属于 #197。
//
// @Summary 项目详情
// @Tags project
// @Produce json
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[ProjectDetailResponse]
// @Router /projects/{id} [get]
func (h *Handler) ProjectDetail(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)

	detail, err := h.svc.ProjectDetail(c.Request.Context(), project.ID(cr.ID),
		h.onlyAdminAuthors())
	if err != nil {
		h.fail(c, err)
		return
	}

	out := ProjectDetailResponse{
		ID:          ctype.ID(detail.Project.ID()),
		Title:       detail.Project.Title(),
		Description: detail.Project.Description(),
		Cover:       detail.Project.Cover(),
		CreatedAt:   detail.Project.CreatedAt(),
		Articles:    make([]ProjectArticleItemResponse, 0, len(detail.Articles)),
	}
	for _, item := range detail.Articles {
		out.Articles = append(out.Articles, ProjectArticleItemResponse{
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

// ── 管理端（#197） ────────────────────────────────────────────
//
// 全部挂在 projectAdminGroup（authMw + AdminMiddleware）下：未授权时由既有的
// 鉴权/管理员中间件返回既有错误码，业务层再兜一次 domain.ErrForbidden。

// AdminProjects 后台项目列表（含未展示的项目）。
//
// 与公开列表的差别：未展示的项目也出现，且带 displayed 与 sort_order，
// 后台据此开关展示与调整项目顺序。
//
// @Summary 后台项目列表（含未展示）
// @Tags project
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[ProjectAdminCard]]
// @Router /projects/admin [get]
func (h *Handler) AdminProjects(c *gin.Context) {
	cards, err := h.svc.ListAdminProjectCards(c.Request.Context(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]ProjectAdminCard, 0, len(cards))
	for _, card := range cards {
		out = append(out, ProjectAdminCard{
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

// CleanupProjectStale 一键清理项目里所有失效的收录行（管理员）。
//
// 只删失效行，仍公开的收录不受影响；没有失效行时返回 removed=0，可重复调用。
// 删的只是收录关系（软删），文章本身不动。
//
// @Summary 清理项目失效行
// @Tags project
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[ProjectStaleCleanupResponse]
// @Router /projects/{id}/admin/stale [delete]
func (h *Handler) CleanupProjectStale(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	removed, err := h.svc.CleanupStaleProjectArticles(c.Request.Context(), h.viewerOf(c),
		project.ID(uri.ID))
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_stale_cleanup", uri.ID, true, "清理项目失效行")
	res.OkWithData(ProjectStaleCleanupResponse{Removed: removed}, c)
}

// CreateProject 新建项目（管理员）。
//
// @Summary 新建项目
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ProjectCreateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[ProjectCreatedResponse]
// @Router /projects [post]
func (h *Handler) CreateProject(c *gin.Context) {
	cr := middleware.GetBindJson[ProjectCreateRequest](c)

	id, err := h.svc.CreateProject(c.Request.Context(), app.CreateProjectCmd{
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
	h.audit(c, "project_create", ctype.ID(id), true, "新建项目")
	res.OkWithData(ProjectCreatedResponse{ID: ctype.ID(id)}, c)
}

// UpdateProject 改项目的标题 / 描述 / 封面（管理员）。
//
// 只改内容：展示开关与顺序各有独立接口，不会因为一次标题编辑被覆盖。
//
// @Summary 改项目
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectUpdateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id} [put]
func (h *Handler) UpdateProject(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectUpdateRequest](c)

	if err := h.svc.UpdateProject(c.Request.Context(), app.UpdateProjectCmd{
		Actor:       h.viewerOf(c),
		ProjectID:   project.ID(uri.ID),
		Title:       cr.Title,
		Description: cr.Description,
		Cover:       cr.Cover,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_update", uri.ID, true, "改项目")
	res.OkWithMsg("保存成功", c)
}

// DeleteProject 软删一个项目（管理员）。
//
// 删项目不删文章：读者侧整块消失，后台列表也不再出现，文章本身照旧。
//
// @Summary 删项目
// @Tags project
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id} [delete]
func (h *Handler) DeleteProject(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	if err := h.svc.DeleteProject(c.Request.Context(), app.DeleteProjectCmd{
		Actor:     h.viewerOf(c),
		ProjectID: project.ID(uri.ID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_delete", uri.ID, true, "删项目")
	res.OkWithMsg("删除成功", c)
}

// SetProjectDisplay 切换项目的展示开关（管理员）。
//
// @Summary 项目展示开关
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectDisplayRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id}/display [patch]
func (h *Handler) SetProjectDisplay(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectDisplayRequest](c)

	if err := h.svc.SetProjectDisplay(c.Request.Context(), app.SetProjectDisplayCmd{
		Actor:     h.viewerOf(c),
		ProjectID: project.ID(uri.ID),
		Displayed: cr.Displayed,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_display", uri.ID, true, "切换项目展示")
	res.OkWithMsg("设置成功", c)
}

// IncludeProjectArticle 把一篇文章收录进项目（管理员）。
//
// @Summary 收录文章进项目
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectArticleRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id}/articles [post]
func (h *Handler) IncludeProjectArticle(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectArticleRequest](c)

	if err := h.svc.IncludeProjectArticle(c.Request.Context(), app.IncludeProjectArticleCmd{
		Actor:     h.viewerOf(c),
		ProjectID: project.ID(uri.ID),
		ArticleID: domain.ArticleID(cr.ArticleID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_include", cr.ArticleID, true, "收录文章进项目")
	res.OkWithMsg("收录成功", c)
}

// RemoveProjectArticle 从项目中移除一篇文章（管理员）。
//
// 幂等：不在项目里也返回成功（后台批量清理时不该因为一行过期数据整单失败）。
//
// @Summary 从项目移除文章
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectArticleRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id}/articles [delete]
func (h *Handler) RemoveProjectArticle(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectArticleRequest](c)

	if err := h.svc.RemoveProjectArticle(c.Request.Context(), app.RemoveProjectArticleCmd{
		Actor:     h.viewerOf(c),
		ProjectID: project.ID(uri.ID),
		ArticleID: domain.ArticleID(cr.ArticleID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_remove", cr.ArticleID, true, "从项目移除文章")
	res.OkWithMsg("移除成功", c)
}

// BatchIncludeProjectArticles 批量收录多篇文章（管理员）。
//
// 与单篇收录的差别：逐篇返回结局（included / duplicated / not_includable /
// missing），而不是整单成败——运营一次勾十几篇时，其中一篇已经收录
// 不该让另外九篇也失败。
//
// @Summary 批量收录文章进项目
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectBatchIncludeRequest true "请求体"
// @Success 200 {object} kernel.Envelope[ProjectBatchIncludeResponse]
// @Router /projects/{id}/articles/batch [post]
func (h *Handler) BatchIncludeProjectArticles(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectBatchIncludeRequest](c)

	ids := make([]domain.ArticleID, 0, len(cr.ArticleIDs))
	for _, id := range cr.ArticleIDs {
		ids = append(ids, domain.ArticleID(id))
	}

	results, err := h.svc.IncludeProjectArticles(c.Request.Context(), app.IncludeProjectArticlesCmd{
		Actor:      h.viewerOf(c),
		ProjectID:  project.ID(uri.ID),
		ArticleIDs: ids,
	})
	if err != nil {
		h.fail(c, err)
		return
	}

	out := ProjectBatchIncludeResponse{
		Results: make([]ProjectBatchIncludeItem, 0, len(results)),
	}
	for _, r := range results {
		out.Results = append(out.Results, ProjectBatchIncludeItem{
			ArticleID: ctype.ID(r.ArticleID),
			Outcome:   string(r.Outcome),
		})
		switch r.Outcome {
		case project.IncludeOutcomeIncluded:
			out.Included++
		case project.IncludeOutcomeDuplicated:
			out.Duplicated++
		default:
			out.Rejected++
		}
	}
	h.audit(c, "project_batch_include", uri.ID, true, "批量收录文章进项目")
	res.OkWithData(out, c)
}

// AdminProjectItems 返回一个项目的全部收录行（含已失效，管理员）。
//
// 后台编排页的读面：全集换序要提交完整集合，失效行也不能漏。
//
// @Summary 后台项目收录行
// @Tags project
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[kernel.ListData[ProjectAdminItem]]
// @Router /projects/{id}/articles/admin [get]
func (h *Handler) AdminProjectItems(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	items, err := h.svc.ListProjectAdminItems(c.Request.Context(),
		h.viewerOf(c), project.ID(uri.ID))
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]ProjectAdminItem, 0, len(items))
	for _, item := range items {
		out = append(out, ProjectAdminItem{
			ArticleID:   ctype.ID(item.ArticleID),
			SortOrder:   item.SortOrder,
			Title:       item.Title,
			Cover:       item.Cover,
			Stale:       item.Stale(),
			StaleReason: string(item.StaleReason()),
		})
	}
	res.OkWithList(out, len(out), c)
}

// ReplaceProjectSortOrder 项目之间的全量顺序替换（管理员）。
//
// 请求体是**完整且有序**的项目 ID 列表；集合与当前全部项目不一致时整单拒绝
// （与项目内排序同一条语义）。落库的是 project_models.sort_order，
// 读者列表页与后台列表都按它排序。
//
// @Summary 项目之间排序
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ProjectOrderRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/order [patch]
func (h *Handler) ReplaceProjectSortOrder(c *gin.Context) {
	cr := middleware.GetBindJson[ProjectOrderRequest](c)

	ids := make([]project.ID, 0, len(cr.ProjectIDs))
	for _, id := range cr.ProjectIDs {
		ids = append(ids, project.ID(id))
	}
	if err := h.svc.ReplaceProjectSortOrder(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_sort_order", "projects", true, "项目之间排序")
	res.OkWithMsg("排序成功", c)
}

// ReplaceProjectOrder 项目内的全量顺序替换（管理员）。
//
// 请求体是**完整且有序**的文章 ID 列表；集合与当前收录不一致时整单拒绝
// （避免过期页面点一次"上移"就把别人刚收录的一篇静默删掉）。
//
// @Summary 项目内文章排序
// @Tags project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "项目 ID"
// @Param body body ProjectArticleOrderRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /projects/{id}/articles/order [patch]
func (h *Handler) ReplaceProjectOrder(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ProjectArticleOrderRequest](c)

	ids := make([]project.ArticleID, 0, len(cr.ArticleIDs))
	for _, id := range cr.ArticleIDs {
		ids = append(ids, project.ArticleID(id))
	}
	if err := h.svc.ReplaceProjectArticleOrder(c.Request.Context(), h.viewerOf(c),
		project.ID(uri.ID), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "project_order", uri.ID, true, "项目内排序")
	res.OkWithMsg("排序成功", c)
}
