// Package http 是社交模块的 HTTP 适配器。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/common/res"
	"myblogx/internal/apperr"
	"myblogx/internal/social/app"
	"myblogx/internal/social/domain"
	"myblogx/utils/jwts"
)

// Handler 是社交 HTTP 处理器。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("social/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

func viewerID(c *gin.Context) uint64 {
	cl := jwts.GetClaimsByGin(c)
	if cl == nil {
		return 0
	}
	return uint64(cl.UserID)
}

// FollowingQuery 关注列表查询。
type FollowingQuery struct {
	Page           int    `form:"page"`
	Limit          int    `form:"limit"`
	UserID         uint64 `form:"user_id"`
	FollowedUserID uint64 `form:"followed_user_id"`
}

// FansQuery 粉丝列表查询。
type FansQuery struct {
	Page       int    `form:"page"`
	Limit      int    `form:"limit"`
	UserID     uint64 `form:"user_id"`
	FansUserID uint64 `form:"fans_user_id"`
}

// Follow 关注用户。
// @Summary 关注用户
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param id path int true "被关注用户 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /follow/{id} [post]
func (h *Handler) Follow(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Follow(c.Request.Context(), viewerID(c), id); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("关注成功", c)
}

// Unfollow 取消关注。
// @Summary 取消关注用户
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param id path int true "被取消关注的用户 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /follow/{id} [delete]
func (h *Handler) Unfollow(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Unfollow(c.Request.Context(), viewerID(c), id); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("取消关注成功", c)
}

// ListFollowing 关注列表。
// @Summary 关注列表
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]domain.FollowingItem,has_more=bool},msg=string}
// @Router /follow [get]
func (h *Handler) ListFollowing(c *gin.Context) {
	var q FollowingQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	list, hasMore, err := h.svc.ListFollowing(c.Request.Context(), viewerID(c), q.UserID, q.FollowedUserID, domain.Page{Page: q.Page, Limit: q.Limit})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// ListFans 粉丝列表。
// @Summary 粉丝列表
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]domain.FansItem,has_more=bool},msg=string}
// @Router /fans [get]
func (h *Handler) ListFans(c *gin.Context) {
	var q FansQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	list, hasMore, err := h.svc.ListFans(c.Request.Context(), viewerID(c), q.UserID, q.FansUserID, domain.Page{Page: q.Page, Limit: q.Limit})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// pathID 解析路径里的数字 ID。
func pathID(c *gin.Context) (uint64, bool) {
	raw := c.Param("id")
	var id uint64
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch < '0' || ch > '9' {
			res.FailWithError(apperr.New(apperr.CodeFollowTargetNotFound), c)
			return 0, false
		}
		id = id*10 + uint64(ch-'0')
	}
	if id == 0 {
		res.FailWithError(apperr.New(apperr.CodeFollowTargetNotFound), c)
		return 0, false
	}
	return id, true
}
