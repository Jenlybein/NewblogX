// Package http 是社交模块的 HTTP 适配器。
//
// 本文件是关系读写的"关系视图"入口（票 327）：关注 / 粉丝 / 互关共用
// GET /relations，计数走 GET /relations/counts，移除粉丝走
// DELETE /relations/fans/{id}。关注与取关仍在 /follow（写的是"我对别人的动作"），
// 读的是"关系的三个视图"，两者不是同一件事，所以不合成一个路径。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/social/domain"

	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// RelationsQuery 关系列表查询。
//
// type 必填：三个视图的语义不同，猜一个默认值只会让调用方拿错数据。
type RelationsQuery struct {
	Page   int      `form:"page"`
	Limit  int      `form:"limit"`
	UserID ctype.ID `form:"user_id"`
	Type   string   `form:"type" binding:"required,oneof=follow fans mutual" enums:"follow,fans,mutual"`
}

// RelationCountsQuery 关系计数查询。
type RelationCountsQuery struct {
	UserID ctype.ID `form:"user_id"`
}

// ListRelations 关系列表（关注 / 粉丝 / 互关）。
// @Summary 关系列表
// @Description type=follow 我关注的人；type=fans 关注我的人；type=mutual 互相关注。
// @Description 互关在服务端过滤（先筛后分页），调用方不需要也不应该自己再筛。
// @Description user_id 省略时看自己的列表；看别人的列表受对方的可见性设置约束。
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param type query string true "视图" Enums(follow,fans,mutual)
// @Param user_id query string false "列表归属用户 ID，缺省为自己"
// @Param page query int false "页码" default(1)
// @Param limit query int false "每页条数" default(10)
// @Success 200 {object} kernel.Envelope[kernel.HasMoreListData[domain.RelationItem]]
// @Router /relations [get]
func (h *Handler) ListRelations(c *gin.Context) {
	var q RelationsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	kind, ok := domain.ParseRelationKind(q.Type)
	if !ok {
		res.FailWithError(apperr.New(apperr.CodeCommonInvalidArgument), c)
		return
	}
	list, hasMore, err := h.svc.ListRelations(c.Request.Context(), viewerID(c), q.UserID.Uint64(), kind,
		domain.Page{Page: q.Page, Limit: q.Limit})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// RelationCounts 关系计数（三个视图各一个数）。
// @Summary 关系计数
// @Description 顶栏徽标用；与列表同一套可见性判据，列表不可见时这里同样失败。
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param user_id query string false "统计归属用户 ID，缺省为自己"
// @Success 200 {object} kernel.Envelope[domain.RelationCounts]
// @Router /relations/counts [get]
func (h *Handler) RelationCounts(c *gin.Context) {
	var q RelationCountsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	counts, err := h.svc.CountRelations(c.Request.Context(), viewerID(c), q.UserID.Uint64())
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(counts, c)
}

// RemoveFan 移除粉丝（单向：不通知对方，也不阻止他再次关注）。
// @Summary 移除粉丝
// @Description 只删"他关注我"这一条。对方不会收到通知，之后仍可再次关注。
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param id path string true "要移除的粉丝用户 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /relations/fans/{id} [delete]
func (h *Handler) RemoveFan(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.RemoveFan(c.Request.Context(), viewerID(c), id); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("已移除该粉丝", c)
}
