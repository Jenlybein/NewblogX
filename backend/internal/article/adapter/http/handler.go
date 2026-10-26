package http

import (
	"context"
	"myblogx/internal/kernel"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/markdown"
)

// ViewerResolver 从请求上下文解析出操作者。
//
// 由装配层注入（复用现有 JWT 鉴权）：HTTP 层负责"你是谁"，
// domain 只关心"你是作者/管理员/路人"。
type ViewerResolver func(c *gin.Context) domain.Viewer

// ImageResolver 是图片模块的只读能力。
//
// 用接口而不是直接拿数据库句柄：HTTP 层不允许出现 SQL
// （archcheck R1 会拦 /adapter/http/ 下的 *gorm.DB）。
type ImageResolver interface {
	// FindImageIDByURL 把图片 URL 反查成图库 ID；不是本站图片时返回 nil。
	FindImageIDByURL(ctx context.Context, url string) (*ctype.ID, error)
	// ResolveImageURLByID 把图库 ID 转成 URL。
	ResolveImageURLByID(ctx context.Context, id ctype.ID) (string, error)
}

// Handler 是 article 模块的 HTTP 处理器。
type Handler struct {
	svc    *app.Service
	logger *logrus.Logger
	site   *sitecfg.RuntimeConfigService
	viewer ViewerResolver
	images ImageResolver
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service, logger *logrus.Logger, site *sitecfg.RuntimeConfigService, viewer ViewerResolver, images ImageResolver) *Handler {
	if viewer == nil {
		viewer = anonymousViewer
	}
	return &Handler{svc: svc, logger: logger, site: site, viewer: viewer, images: images}
}

// anonymousViewer 是兜底：没有鉴权信息时一律当匿名。
func anonymousViewer(*gin.Context) domain.Viewer { return domain.Anonymous() }

// viewerOf 取当前操作者。
func (h *Handler) viewerOf(c *gin.Context) domain.Viewer { return h.viewer(c) }

// skipExamining 取站点配置"是否跳过审核"。
func (h *Handler) skipExamining() bool {
	if h.site == nil {
		return false
	}
	return h.site.GetRuntimeArticle().SkipExamining
}

// fail 统一错误出口：把领域错误码翻成 HTTP 响应。
//
// 不区分"不存在"与"无权"：两者在 domain 里本来就合并成 ErrArticleNotFound，
// 防止用响应差异探测他人草稿。
func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case err == nil:
		res.FailWithMsg("操作失败", c)
	default:
		res.FailWithMsg(err.Error(), c)
	}
}

// ── 详情 ──

// Detail 查询文章详情。
//
// @Summary 文章详情
// @Tags article
// @Produce json
// @Param id path string true "文章 ID"
// @Success 200 {object} kernel.Envelope[ArticleDetailResponse]
// @Router /articles/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)

	view, err := h.svc.Detail(c.Request.Context(), cr.ID, h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(h.toDetailResponse(c.Request.Context(), view), c)
}

// ── 创建 ──

// Create 创建文章。
//
// @Summary 创建文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ArticleCreateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[ArticleCreateResponse]
// @Router /articles [post]
func (h *Handler) Create(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleCreateRequest](c)
	v := h.viewerOf(c)

	content := markdown.MdToSafe(cr.Content)
	abstract := cr.Abstract
	if abstract == "" {
		abstract = markdown.ExtractText(markdown.MdToText(content), 200)
	}

	coverURL, coverImageID, err := h.resolveCover(c.Request.Context(), cr.CoverImageID)
	if err != nil {
		h.fail(c, err)
		return
	}

	article, err := h.svc.Create(c.Request.Context(), app.CreateArticleCmd{
		Actor:          v,
		Title:          cr.Title,
		Abstract:       abstract,
		Content:        content,
		CategoryID:     toDomainCategoryID(cr.CategoryID),
		TagIDs:         toDomainTagIDs(cr.TagIDs),
		Cover:          coverURL,
		CommentsToggle: cr.CommentsToggle,
		TargetStatus:   domain.Status(cr.Status),
		Visibility:     domain.Visibility(cr.VisibilityStatus),
		SkipExamining:  h.skipExamining(),
	})
	if err != nil {
		h.fail(c, err)
		return
	}

	h.audit(c, "article_create", article.ID(), true, "创建文章")
	res.OkWithData(toCreateResponse(app.ArticleView{
		Article: *article,
		Tags:    nil,
	}, coverImageID), c)
}

// ── 更新 ──

// Update 更新文章。
//
// @Summary 更新文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Param body body ArticleUpdateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[ArticleDetailResponse]
// @Router /articles/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)
	cr := middleware.GetBindJson[ArticleUpdateRequest](c)

	var content *string
	if cr.Content != nil {
		safe := markdown.MdToSafe(*cr.Content)
		content = &safe
	}

	var abstract *string
	if cr.Abstract != nil {
		a := *cr.Abstract
		if a == "" && content != nil {
			a = markdown.ExtractText(markdown.MdToText(*content), 200)
		}
		abstract = &a
	}

	var cover *string
	if cr.CoverImageID != nil && h.images != nil {
		url, err := h.images.ResolveImageURLByID(c.Request.Context(), *cr.CoverImageID)
		if err != nil {
			h.fail(c, err)
			return
		}
		cover = &url
	}

	var tags *[]domain.TagID
	if cr.TagIDs != nil {
		ids := toDomainTagIDs(*cr.TagIDs)
		tags = &ids
	}

	var target *domain.Status
	if cr.Status != nil {
		st := domain.Status(*cr.Status)
		target = &st
	}

	var visibility *domain.Visibility
	if cr.VisibilityStatus != nil {
		vv := domain.Visibility(*cr.VisibilityStatus)
		visibility = &vv
	}

	_, err := h.svc.Update(c.Request.Context(), app.UpdateArticleCmd{
		Actor:          h.viewerOf(c),
		ArticleID:      uri.ID,
		Title:          cr.Title,
		Abstract:       abstract,
		Content:        content,
		CategoryID:     toDomainCategoryID(cr.CategoryID),
		CategorySet:    cr.CategoryID != nil,
		Cover:          cover,
		CommentsToggle: cr.CommentsToggle,
		Visibility:     visibility,
		TagIDs:         tags,
		TargetStatus:   target,
		SkipExamining:  h.skipExamining(),
	})
	if err != nil {
		h.fail(c, err)
		return
	}

	h.audit(c, "article_update", uri.ID, true, "更新文章")
	view, err := h.svc.Detail(c.Request.Context(), uri.ID, h.viewerOf(c))
	if err != nil {
		res.OkWithMsg("更新成功", c)
		return
	}
	res.OkWithData(h.toDetailResponse(c.Request.Context(), view), c)
}

// ── 删除 ──

// Remove 作者删除自己的文章。
//
// @Summary 删除文章
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/{id} [delete]
func (h *Handler) Remove(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	if _, err := h.svc.Delete(c.Request.Context(), app.DeleteCmd{
		Actor:     h.viewerOf(c),
		ArticleID: uri.ID,
	}); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "article_delete", uri.ID, true, "删除文章")
	res.OkWithMsg("删除成功", c)
}

// RemoveBatch 管理员批量删除。
//
// @Summary 批量删除文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body kernel.IDListRequest true "文章 ID 列表"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles [delete]
func (h *Handler) RemoveBatch(c *gin.Context) {
	cr := middleware.GetBindJson[kernel.IDListRequest](c)

	deleted := 0
	for i := range cr.IDList {
		if _, err := h.svc.Delete(c.Request.Context(), app.DeleteCmd{
			Actor:     h.viewerOf(c),
			ArticleID: cr.IDList[i],
		}); err != nil {
			h.logger.Warnf("批量删除文章 %s 失败: %v", cr.IDList[i], err)
			continue
		}
		deleted++
	}
	h.audit(c, "article_delete_batch", "", true, "批量删除文章")
	res.OkWithMsg("删除成功", c)
}

// ── 可见性 ──

// AdminVisibility 管理员切换可见性。
//
// @Summary 管理员设置文章可见性
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Param visibility path string true "hide 或 show"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/{id}/admin/{visibility} [post]
func (h *Handler) AdminVisibility(c *gin.Context) {
	uri := middleware.GetBindUri[ArticleAdminVisibilityURI](c)

	next := domain.VisibilityAdminHidden
	if uri.Visibility == "show" {
		next = domain.VisibilityVisible
	}
	if _, err := h.svc.SetVisibility(c.Request.Context(), h.viewerOf(c), domain.ArticleID(uri.ID), next); err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, "article_admin_visibility", domain.ArticleID(uri.ID), true, "设置文章可见性")
	res.OkWithMsg("设置成功", c)
}

// ── 浏览 ──

// Visit 记录浏览。
//
// @Summary 记录文章浏览
// @Tags article
// @Accept json
// @Produce json
// @Param body body ArticleViewCountRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /articles/view [post]
func (h *Handler) Visit(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleViewCountRequest](c)

	if err := h.svc.RecordView(c.Request.Context(), domain.ArticleID(cr.ArticleID), h.viewerOf(c)); err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithMsg("记录成功", c)
}

// ── 点赞 ──

// Digg 点赞/取消点赞。
//
// @Summary 文章点赞
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param id path string true "文章 ID"
// @Success 200 {object} kernel.Envelope[ArticleDiggResponse]
// @Router /articles/{id}/digg [put]
func (h *Handler) Digg(c *gin.Context) {
	uri := middleware.GetBindUri[kernel.IDRequest](c)

	liked, err := h.svc.Digg(c.Request.Context(), uri.ID, h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(ArticleDiggResponse{IsDigg: liked}, c)
}

// ── 内部助手 ──

// resolveCover 把图片 ID 转成 URL，同时保留原始 ID 供响应回显。
func (h *Handler) resolveCover(ctx context.Context, imageID *ctype.ID) (string, *ctype.ID, error) {
	if imageID == nil {
		return "", nil, nil
	}
	if h.images == nil {
		return "", nil, nil
	}
	url, err := h.images.ResolveImageURLByID(ctx, *imageID)
	if err != nil {
		return "", nil, err
	}
	return url, imageID, nil
}

// audit 写审计日志。
//
// 覆盖范围：16 个写接口全部调用（S3.9 的验收口径）。
// 记录的是"谁对哪篇文章做了什么、结果如何"，不含请求体（L.10 决策）。
func (h *Handler) audit(c *gin.Context, action string, id domain.ArticleID, success bool, msg string) {
	targetID := ""
	if !id.IsZero() {
		targetID = strconv.FormatUint(id.Uint64(), 10)
	}
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: action,
		TargetType: "article",
		TargetID:   targetID,
		Success:    success,
		Message:    msg,
	})
}

func toDomainCategoryID(id *ctype.ID) *domain.CategoryID {
	if id == nil {
		return nil
	}
	v := domain.CategoryID(*id)
	return &v
}

func toDomainTagIDs(ids []ctype.ID) []domain.TagID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]domain.TagID, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.TagID(id))
	}
	return out
}
