// Package http 是社交模块的 HTTP 适配器。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/social/app"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
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
	return cl.UserID.Uint64()
}

// Follow 关注用户。
// @Summary 关注用户
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param id path string true "被关注用户 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
// @Param id path string true "被取消关注的用户 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
