// Package http 是评论模块的 HTTP 适配器。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"
)

// Handler 是评论 HTTP 处理器。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("comment/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// viewer 取当前登录态；匿名返回零值。
func viewer(c *gin.Context) domain.Viewer {
	cl := jwts.GetClaimsByGin(c)
	if cl == nil {
		return domain.Viewer{}
	}
	return domain.Viewer{ID: uint64(cl.UserID), Admin: cl.IsAdmin()}
}

// CreateRequest 发表评论。
type CreateRequest struct {
	Content   string  `json:"content" binding:"required"`
	ArticleID uint64  `json:"article_id" binding:"required"`
	ReplyID   *uint64 `json:"reply_id"`
}

// RootListRequest 一级评论列表。
type RootListRequest struct {
	Page      int    `form:"page"`
	Limit     int    `form:"limit"`
	Key       string `form:"key"`
	ArticleID uint64 `form:"article_id" binding:"required"`
}

// ReplyListRequest 二级评论列表。
type ReplyListRequest struct {
	Page      int    `form:"page"`
	Limit     int    `form:"limit"`
	Key       string `form:"key"`
	ArticleID uint64 `form:"article_id" binding:"required"`
	RootID    uint64 `form:"root_id" binding:"required"`
}

// ManListRequest 管理端评论列表。
type ManListRequest struct {
	Page      int    `form:"page"`
	Limit     int    `form:"limit"`
	Key       string `form:"key"`
	ArticleID uint64 `form:"article_id"`
	UserID    uint64 `form:"user_id"`
	Status    int8   `form:"status"`
	Type      int8   `form:"type" binding:"required"`
}

func pageOf(p, limit int, key string) domain.Page {
	return domain.Page{Page: p, Limit: limit, Key: key}
}

// Create 发表评论或回复。
// @Summary 发表评论
// @Tags comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateRequest true "请求体"
// @Success 200 {object} object{code=int,data=app.CreateResult,msg=string}
// @Router /comments [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	result, err := h.svc.Create(c.Request.Context(), viewer(c), app.CreateInput{
		Content:   req.Content,
		ArticleID: req.ArticleID,
		ReplyID:   req.ReplyID,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(result, c)
}

// Digg 点赞 / 取消点赞。
// @Summary 评论点赞/取消点赞
// @Tags comment
// @Produce json
// @Security BearerAuth
// @Param id path int true "评论 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /comments/{id}/digg [post]
func (h *Handler) Digg(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	digged, err := h.svc.ToggleDigg(c.Request.Context(), viewer(c), id)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	if digged {
		res.OkWithMsg("点赞成功", c)
		return
	}
	res.OkWithMsg("取消点赞成功", c)
}

// Remove 删除评论。
// @Summary 删除评论
// @Tags comment
// @Produce json
// @Security BearerAuth
// @Param id path int true "评论 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /comments/{id} [delete]
func (h *Handler) Remove(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Remove(c.Request.Context(), viewer(c), id); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("删除评论成功", c)
}

// ListRoot 一级评论列表。
// @Summary 一级评论列表
// @Tags comment
// @Produce json
// @Param article_id query int true "文章 ID"
// @Success 200 {object} object{code=int,data=object{list=[]app.RootItem,has_more=bool},msg=string}
// @Router /comments [get]
func (h *Handler) ListRoot(c *gin.Context) {
	var req RootListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	items, hasMore, err := h.svc.ListRoot(c.Request.Context(), req.ArticleID, pageOf(req.Page, req.Limit, req.Key), viewer(c).ID)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(map[string]any{"list": items, "has_more": hasMore}, c)
}

// ListReplies 二级评论列表。
// @Summary 二级评论列表
// @Tags comment
// @Produce json
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /comments/replies [get]
func (h *Handler) ListReplies(c *gin.Context) {
	var req ReplyListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	items, replyCount, hasMore, err := h.svc.ListReplies(c.Request.Context(), req.ArticleID, req.RootID, pageOf(req.Page, req.Limit, req.Key), viewer(c).ID)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(map[string]any{
		"root_id":     req.RootID,
		"reply_count": replyCount,
		"list":        items,
		"has_more":    hasMore,
	}, c)
}

// ListManaged 管理端评论列表。
// @Summary 评论管理列表
// @Tags comment
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]app.ManageItem,count=int},msg=string}
// @Router /comments/man [get]
func (h *Handler) ListManaged(c *gin.Context) {
	var req ManListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	items, total, err := h.svc.ListManaged(c.Request.Context(), viewer(c), app.ManagedInput{
		Type:      req.Type,
		Status:    req.Status,
		ArticleID: req.ArticleID,
		UserID:    req.UserID,
		Page:      pageOf(req.Page, req.Limit, req.Key),
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(items, int(total), c)
}

// pathID 解析路径里的数字 ID。
func pathID(c *gin.Context) (uint64, bool) {
	raw := c.Param("id")
	var id uint64
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch < '0' || ch > '9' {
			res.FailWithMsg("参数错误", c)
			return 0, false
		}
		id = id*10 + uint64(ch-'0')
	}
	if id == 0 {
		res.FailWithMsg("参数错误", c)
		return 0, false
	}
	return id, true
}
