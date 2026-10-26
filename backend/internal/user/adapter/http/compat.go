package http

import (
	"myblogx/internal/kernel/ctype"
	"strconv"

	"github.com/gin-gonic/gin"

	"myblogx/internal/platform/http/res"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// 本文件是旧路径的兼容 handler：路径与字段不变，内部换成新用例。

// BaseInfo 指定用户的主页基础信息。
// @Summary 用户基础信息
// @Tags user
// @Produce json
// @Param id query string true "ID"
// @Success 200 {object} kernel.Envelope[BaseInfoResponse]
// @Router /users/base [get]
func (h *Handler) BaseInfo(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || id == 0 {
		res.FailWithMsg("参数错误", c)
		return
	}
	p, err := h.svc.BaseInfo(c.Request.Context(), viewer(c).ID, ctype.NewID(id))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(BaseInfoResponse{
		ID:                  p.ID,
		CodeAge:             p.CodeAge,
		AvatarImageID:       p.AvatarImageID,
		Avatar:              p.Avatar,
		Nickname:            p.Nickname,
		Abstract:            p.Abstract,
		ViewCount:           p.Stats.ViewCount,
		FansCount:           p.Stats.FansCount,
		FollowCount:         p.Stats.FollowCount,
		ArticleVisitedCount: p.Stats.ArticleVisitedCount,
		ArticleCount:        p.Stats.ArticleCount,
		FavoritesVisibility: p.FavoritesVis,
		FollowVisibility:    p.FollowVis,
		FansVisibility:      p.FansVis,
		HomeStyleID:         p.HomeStyleID,
		Relation:            p.Relation,
		Place:               p.Place,
	}, c)
}

// UpdateInfo 更新当前用户的资料与偏好。
// @Summary 更新个人信息
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body UpdateInfoRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/info [put]
func (h *Handler) UpdateInfo(c *gin.Context) {
	var req UpdateInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	if _, err := h.svc.UpdateInfo(c.Request.Context(), viewer(c), userID(c), req.toApp()); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("用户信息更新成功", c)
}

// AdminUpdate 管理员更新指定用户信息。
// @Summary 管理员更新用户信息
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body AdminUpdateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/admin/info [put]
func (h *Handler) AdminUpdate(c *gin.Context) {
	var req AdminUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	patch := app.AdminPatchInput{
		Username:      req.Username,
		Nickname:      req.Nickname,
		Abstract:      req.Abstract,
		AvatarImageID: req.AvatarImageID,
	}
	if req.Role != nil {
		role := domain.Role(*req.Role)
		patch.Role = &role
	}
	if req.Status != nil {
		status := domain.Status(*req.Status)
		patch.Status = &status
	}
	if err := h.svc.AdminUpdate(c.Request.Context(), viewer(c), req.UserID, patch); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("用户信息更新成功", c)
}

// AdminList 管理端用户列表。
// @Summary 用户列表
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[AdminUserItemResponse]]
// @Router /users/admin/list [get]
func (h *Handler) AdminList(c *gin.Context) {
	var req AdminListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	items, total, err := h.svc.AdminList(c.Request.Context(), viewer(c), domain.Query{
		Keyword: req.Key,
		Page:    req.Page,
		Size:    req.Limit,
	})
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]AdminUserItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, AdminUserItemResponse{
			ID:          it.ID,
			Nickname:    it.Nickname,
			Avatar:      it.Avatar,
			Username:    it.Username,
			CreatedAt:   it.CreatedAt,
			IP:          it.IP,
			Addr:        it.Addr,
			LastLoginAt: it.LastLoginAt,
		})
	}
	res.OkWithList(out, int(total), c)
}

// AdminCreate 管理员创建用户。
// @Summary 管理员创建用户
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body AdminCreateRequest true "请求体"
// @Success 200 {object} kernel.Envelope[AdminCreateResponse]
// @Router /users/admin [post]
func (h *Handler) AdminCreate(c *gin.Context) {
	var req AdminCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}
	var email *string
	if req.Email != "" {
		value := req.Email
		email = &value
	}
	u, err := h.svc.Create(c.Request.Context(), viewer(c), app.CreateInput{
		Username: req.Username,
		Nickname: nickname,
		Password: req.Password,
		Email:    email,
		IP:       c.ClientIP(),
	})
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(AdminCreateResponse{
		ID:             u.ID,
		Username:       u.Username,
		Nickname:       u.Nickname,
		Email:          u.Email,
		Role:           int8(u.Role),
		Status:         int8(u.Status),
		RegisterSource: int8(u.RegisterSource),
	}, c)
}
