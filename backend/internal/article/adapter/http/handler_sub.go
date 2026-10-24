package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/common/res"
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// ── 分类 ──

// Categories 分类列表。
//
// @Summary 分类列表
// @Tags article
// @Produce json
// @Param user_id query int false "用户 ID"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/category [get]
func (h *Handler) Categories(c *gin.Context) {
	cr := middleware.GetBindQuery[CategoryListRequest](c)

	list, err := h.svc.ListCategories(c.Request.Context(), h.viewerOf(c), domain.UserID(cr.UserID))
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(list, len(list), c)
}

// SaveCategory 创建或更新分类。
//
// @Summary 保存分类
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CategoryRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/category [post]
func (h *Handler) SaveCategory(c *gin.Context) {
	cr := middleware.GetBindJson[CategoryRequest](c)

	if _, err := h.svc.SaveCategory(c.Request.Context(), h.viewerOf(c), domain.CategoryID(cr.ID), cr.Title); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "category_save", 0, true, "保存分类")
	res.OkWithMsg("保存成功", c)
}

// DeleteCategories 删除分类。
//
// @Summary 删除分类
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "分类 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/category [delete]
func (h *Handler) DeleteCategories(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	ids := make([]domain.CategoryID, 0, len(cr.IDList))
	for _, id := range cr.IDList {
		ids = append(ids, domain.CategoryID(id))
	}
	if _, err := h.svc.DeleteCategories(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "category_delete", 0, true, "删除分类")
	res.OkWithMsg("删除成功", c)
}

// CategoryOptions 分类下拉项。
//
// @Summary 分类下拉
// @Tags article
// @Produce json
// @Param user_id query int false "用户 ID"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/category/options [get]
func (h *Handler) CategoryOptions(c *gin.Context) {
	h.Categories(c)
}

// ── 标签 ──

// Tags 标签列表。
//
// @Summary 标签列表
// @Tags article
// @Produce json
// @Param include_disabled query bool false "是否包含停用"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/tags [get]
func (h *Handler) Tags(c *gin.Context) {
	cr := middleware.GetBindQuery[TagListRequest](c)

	list, err := h.svc.ListTags(c.Request.Context(), cr.IncludeDisabled)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(list, len(list), c)
}

// TagOptions 标签下拉项（只含启用项）。
//
// @Summary 标签下拉
// @Tags article
// @Produce json
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/tags/options [get]
func (h *Handler) TagOptions(c *gin.Context) {
	list, err := h.svc.ListTags(c.Request.Context(), false)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(list, len(list), c)
}

// SaveTag 创建或更新标签（管理员）。
//
// @Summary 保存标签
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body TagRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/tags [put]
func (h *Handler) SaveTag(c *gin.Context) {
	cr := middleware.GetBindJson[TagRequest](c)

	if _, err := h.svc.SaveTag(c.Request.Context(), h.viewerOf(c), app.TagInput{
		ID:          domain.TagID(cr.ID),
		Title:       cr.Title,
		Sort:        cr.Sort,
		Description: cr.Description,
		Enabled:     cr.IsEnabled,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "tag_save", 0, true, "保存标签")
	res.OkWithMsg("保存成功", c)
}

// DeleteTags 删除标签（管理员）。
//
// @Summary 删除标签
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "标签 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/tags [delete]
func (h *Handler) DeleteTags(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	ids := make([]domain.TagID, 0, len(cr.IDList))
	for _, id := range cr.IDList {
		ids = append(ids, domain.TagID(id))
	}
	if _, err := h.svc.DeleteTags(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "tag_delete", 0, true, "删除标签")
	res.OkWithMsg("删除成功", c)
}

// ── 收藏夹 ──

// Folders 收藏夹列表。
//
// @Summary 收藏夹列表
// @Tags article
// @Produce json
// @Param user_id query int false "用户 ID"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/favorite [get]
func (h *Handler) Folders(c *gin.Context) {
	cr := middleware.GetBindQuery[FavoriteListRequest](c)

	target := domain.UserID(cr.UserID)
	if target.IsZero() {
		target = h.viewerOf(c).UserID
	}
	list, err := h.svc.ListFolders(c.Request.Context(), target)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(list, len(list), c)
}

// SaveFolder 创建或更新收藏夹。
//
// @Summary 保存收藏夹
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body FavoriteRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/favorite [put]
func (h *Handler) SaveFolder(c *gin.Context) {
	cr := middleware.GetBindJson[FavoriteRequest](c)

	if _, err := h.svc.SaveFolder(c.Request.Context(), h.viewerOf(c), app.FolderInput{
		ID:       favorite.ID(cr.ID),
		Title:    cr.Title,
		Cover:    cr.Cover,
		Abstract: cr.Abstract,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "favorite_folder_save", 0, true, "保存收藏夹")
	res.OkWithMsg("保存成功", c)
}

// DeleteFolders 删除收藏夹。
//
// @Summary 删除收藏夹
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "收藏夹 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/favorite [delete]
func (h *Handler) DeleteFolders(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	ids := make([]favorite.ID, 0, len(cr.IDList))
	for _, id := range cr.IDList {
		ids = append(ids, favorite.ID(id))
	}
	if _, err := h.svc.DeleteFolders(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "favorite_folder_delete", 0, true, "删除收藏夹")
	res.OkWithMsg("删除成功", c)
}

// FavoriteItems 收藏夹内文章。
//
// @Summary 收藏夹内容
// @Tags article
// @Produce json
// @Param favor_id query int true "收藏夹 ID"
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/favorite/contents [get]
func (h *Handler) FavoriteItems(c *gin.Context) {
	cr := middleware.GetBindQuery[FavoriteArticlesRequest](c)

	items, total, err := h.svc.ListFavoriteItems(c.Request.Context(), h.viewerOf(c),
		favorite.ID(cr.FavorID), domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(items, int(total), c)
}

// SaveFavorite 收藏文章。
//
// @Summary 收藏文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ArticleFavoriteRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/favorite [post]
func (h *Handler) SaveFavorite(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleFavoriteRequest](c)

	if err := h.svc.Favorite(c.Request.Context(), app.FavoriteCmd{
		Actor:     h.viewerOf(c),
		ArticleID: domain.ArticleID(cr.ArticleID),
		FolderID:  favorite.ID(cr.FavorID),
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "article_favorite", domain.ArticleID(cr.ArticleID), true, "收藏文章")
	res.OkWithMsg("收藏成功", c)
}

// RemoveFavoriteItems 批量移除收藏。
//
// @Summary 取消收藏
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body FavoriteRemovePatchModel true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/favorite/contents [delete]
func (h *Handler) RemoveFavoriteItems(c *gin.Context) {
	cr := middleware.GetBindJson[FavoriteRemovePatchModel](c)

	for _, id := range cr.ArticleIDs {
		if err := h.svc.Unfavorite(c.Request.Context(), app.FavoriteCmd{
			Actor:     h.viewerOf(c),
			ArticleID: domain.ArticleID(id),
			FolderID:  favorite.ID(cr.FavorID),
		}); err != nil {
			h.logger.Warnf("取消收藏文章 %d 失败: %v", id, err)
		}
	}
	h.audit(c, "article_unfavorite", 0, true, "取消收藏")
	res.OkWithMsg("取消成功", c)
}

// ── 置顶 ──

// Tops 置顶列表。
//
// @Summary 置顶列表
// @Tags article
// @Produce json
// @Param user_id query int false "用户 ID"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/top [get]
func (h *Handler) Tops(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleTopListRequest](c)

	target := domain.UserID(cr.UserID)
	if target.IsZero() {
		target = h.viewerOf(c).UserID
	}
	ids, err := h.svc.ListTops(c.Request.Context(), target)
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(ids, len(ids), c)
}

// SetTop 批量置顶。
//
// @Summary 设置置顶
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ArticleTopSetRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/top [post]
func (h *Handler) SetTop(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleTopSetRequest](c)
	h.toggleTops(c, cr.ArticleIDs, false)
}

// RemoveTop 批量取消置顶。
//
// @Summary 取消置顶
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ArticleTopSetRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/top [delete]
func (h *Handler) RemoveTop(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleTopSetRequest](c)
	h.toggleTops(c, cr.ArticleIDs, true)
}

func (h *Handler) toggleTops(c *gin.Context, ids []ctype.ID, remove bool) {
	failed := 0
	for _, id := range ids {
		if err := h.svc.Top(c.Request.Context(), app.TopCmd{
			Actor:     h.viewerOf(c),
			ArticleID: domain.ArticleID(id),
			Remove:    remove,
		}); err != nil {
			failed++
			h.logger.Warnf("置顶操作失败 article=%d remove=%v: %v", id, remove, err)
		}
	}
	h.audit(c, "article_top", 0, failed == 0, "设置置顶")
	if failed > 0 && failed == len(ids) {
		res.FailWithMsg("设置置顶失败", c)
		return
	}
	res.OkWithMsg("设置成功", c)
}

// ── 浏览历史 ──

// History 浏览历史列表。
//
// @Summary 浏览历史
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} object{code=int,data=object{list=[]object{},count=int},msg=string}
// @Router /articles/history [get]
func (h *Handler) History(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleViewHistoryRequest](c)

	records, total, err := h.svc.ListViewHistory(c.Request.Context(), h.viewerOf(c),
		domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithList(records, int(total), c)
}

// RemoveHistory 删除浏览历史。
//
// @Summary 删除浏览历史
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "文章 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/history [delete]
func (h *Handler) RemoveHistory(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	ids := make([]domain.ArticleID, 0, len(cr.IDList))
	for _, id := range cr.IDList {
		ids = append(ids, domain.ArticleID(id))
	}
	if err := h.svc.RemoveViewHistory(c.Request.Context(), h.viewerOf(c), ids); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "view_history_delete", 0, true, "删除浏览历史")
	res.OkWithMsg("删除成功", c)
}

// ── 审核任务 ──

// ReviewTasks 审核任务列表。
//
// @Summary 审核任务列表
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param status query string false "任务状态"
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} object{code=int,data=ArticleReviewTaskListResponse,msg=string}
// @Router /article-review [get]
func (h *Handler) ReviewTasks(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleReviewTaskListRequest](c)

	var status *review.Status
	if cr.Status != "" {
		s := review.Status(cr.Status)
		status = &s
	}

	tasks, total, err := h.svc.ListReviewTasks(c.Request.Context(), h.viewerOf(c), status,
		domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}

	items := make([]ArticleReviewTaskItem, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toTaskItem(t))
	}
	res.OkWithData(ArticleReviewTaskListResponse{Count: total, List: items}, c)
}

// HandleReviewTask 处理审核任务。
//
// @Summary 处理审核任务
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Param body body ArticleReviewHandleRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /article-review-tasks/{id}/review [post]
func (h *Handler) HandleReviewTask(c *gin.Context) {
	uri := middleware.GetBindUri[models.IDRequest](c)
	cr := middleware.GetBindJson[ArticleReviewHandleRequest](c)

	approve := cr.Status == enum.ArticleStatusPublished
	if _, err := h.svc.HandleReviewTask(c.Request.Context(), h.viewerOf(c), uint64(uri.ID), approve, cr.Reason); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "article_review_handle", 0, true, "处理审核任务")
	res.OkWithMsg("处理成功", c)
}

// ── 作者信息 ──

// AuthorInfo 作者信息。
//
// @Summary 作者信息
// @Tags article
// @Produce json
// @Param author_id query int true "作者 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/author_info [get]
func (h *Handler) AuthorInfo(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleAuthorInfoBindRequest](c)

	// 作者信息本身是公开数据；只返回展示所需字段，
	// 不暴露邮箱等敏感信息（旧实现同样只返回这些）。
	profile, err := h.svc.AuthorProfile(c.Request.Context(), domain.UserID(cr.AuthorID))
	if err != nil {
		h.fail(c, err)
		return
	}
	if profile.ID.IsZero() {
		res.FailWithMsg("用户不存在", c)
		return
	}
	res.OkWithData(map[string]any{
		"id":       profile.ID.Uint64(),
		"username": profile.Username,
		"nickname": profile.Nickname,
		"avatar":   profile.Avatar,
	}, c)
}

// pageOffset 把 1-based 页码转成 offset。
//
// 旧接口用 page/page_size；为空时取第 1 页、每页 10。
func pageOffset(page, limit int) int {
	if limit <= 0 {
		limit = 10
	}
	if page <= 1 {
		return 0
	}
	return (page - 1) * limit
}
