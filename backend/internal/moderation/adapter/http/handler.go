// Package http 是违规计分与申诉的 HTTP 适配器（ADR-0035）。
package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	moderationapp "myblogx/internal/moderation/app"
	moderationdomain "myblogx/internal/moderation/domain"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"

	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 是计分与申诉的处理器。
type Handler struct {
	svc *moderationapp.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *moderationapp.Service) *Handler { return &Handler{svc: svc} }

// EventItem 是一条违规事件（出站形状）。
type EventItem struct {
	ID        uint64 `json:"id"`
	Level     string `json:"level" enums:"keyword,image,general,severe"`
	Score     int    `json:"score"`
	Source    string `json:"source" enums:"machine,manual"`
	RefType   string `json:"ref_type"`
	RefID     uint64 `json:"ref_id"`
	Evidence  string `json:"evidence"`
	Status    string `json:"status" enums:"active,revoked"`
	CreatedAt string `json:"created_at"`
	// CanAppeal 是后端算好的"现在还能不能申诉"：窗口判断只在服务端做一次，
	// 前端不重复实现时间规则。
	CanAppeal bool `json:"can_appeal"`
}

// SummaryResponse 是"我的违规记录"。
type SummaryResponse struct {
	UserID uint64 `json:"user_id"`
	// Score 是有效分（保留一位小数）：衰减是连续函数，取整会让用户对不上账。
	Score      float64 `json:"score"`
	Punishment string  `json:"punishment" enums:"none,mute_1d,mute_7d,mute_30d,manual_review"`
	// PunishmentLabel 是给用户看的中文档位说明。
	PunishmentLabel string      `json:"punishment_label"`
	Items           []EventItem `json:"items"`
}

// AppealRequest 提交申诉。
type AppealRequest struct {
	EventID uint64 `json:"event_id" binding:"required"`
	Reason  string `json:"reason" binding:"required,max=512"`
}

// AppealItem 一条申诉（出站形状）。
type AppealItem struct {
	ID        uint64 `json:"id"`
	EventID   uint64 `json:"event_id"`
	UserID    uint64 `json:"user_id"`
	Reason    string `json:"reason"`
	Status    string `json:"status" enums:"pending,approved,rejected"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

// HandleAppealRequest 处置申诉。
type HandleAppealRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note" binding:"max=512"`
}

// MySummary 读自己的有效分与违规事件时间线。
// @Summary 我的违规记录
// @Tags moderation
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SummaryResponse]
// @Router /moderation/me [get]
func (h *Handler) MySummary(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(c.Request.Context(), userID)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(toSummaryResponse(summary), c)
}

// SubmitAppeal 提交申诉。
// @Summary 提交申诉
// @Tags moderation
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body AppealRequest true "申诉内容"
// @Success 200 {object} kernel.Envelope[AppealItem]
// @Router /moderation/me/appeals [post]
func (h *Handler) SubmitAppeal(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req AppealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	appeal, err := h.svc.SubmitAppeal(c.Request.Context(), userID, req.EventID, req.Reason)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(toAppealItem(appeal), c)
}

// ListAppeals 申诉列表（管理员）。
// @Summary 申诉列表
// @Tags moderation
// @Produce json
// @Security BearerAuth
// @Param status query string false "状态筛选：pending / approved / rejected"
// @Success 200 {object} kernel.Envelope[kernel.ListData[AppealItem]]
// @Router /moderation/appeals [get]
func (h *Handler) ListAppeals(c *gin.Context) {
	status := moderationdomain.AppealStatus(strings.TrimSpace(c.Query("status")))
	switch status {
	case "", moderationdomain.AppealPending, moderationdomain.AppealApproved, moderationdomain.AppealRejected:
	default:
		res.FailWithMsg("申诉状态不存在（可选：pending / approved / rejected）", c)
		return
	}
	appeals, err := h.svc.ListAppeals(c.Request.Context(), status)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	items := make([]AppealItem, 0, len(appeals))
	for i := range appeals {
		items = append(items, toAppealItem(&appeals[i]))
	}
	res.OkWithList(items, len(items), c)
}

// HandleAppeal 处置申诉（管理员）。
// @Summary 处置申诉
// @Tags moderation
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "申诉 ID"
// @Param body body HandleAppealRequest true "处置结果"
// @Success 200 {object} kernel.Envelope[AppealItem]
// @Router /moderation/appeals/{id}/handle [post]
func (h *Handler) HandleAppeal(c *gin.Context) {
	operator, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req HandleAppealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	appeal, err := h.svc.HandleAppeal(c.Request.Context(), id, operator, req.Approve, req.Note)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(toAppealItem(appeal), c)
}

// toSummaryResponse 领域概览 → 出站形状。
func toSummaryResponse(summary *moderationapp.Summary) SummaryResponse {
	items := make([]EventItem, 0, len(summary.Events))
	for i := range summary.Events {
		event := summary.Events[i]
		items = append(items, EventItem{
			ID:        event.ID,
			Level:     string(event.Level),
			Score:     event.Score,
			Source:    string(event.Source),
			RefType:   event.RefType,
			RefID:     event.RefID,
			Evidence:  event.Evidence,
			Status:    string(event.Status),
			CreatedAt: event.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			// 申诉窗口服务端算：前端不重复实现时间规则，避免两边判断不一致。
			CanAppeal: event.Status == moderationdomain.StatusActive,
		})
	}
	return SummaryResponse{
		UserID:          summary.UserID,
		Score:           summary.Score,
		Punishment:      string(summary.Punishment),
		PunishmentLabel: summary.Punishment.LadderLabel(),
		Items:           items,
	}
}

// toAppealItem 领域申诉 → 出站形状。
func toAppealItem(appeal *moderationdomain.Appeal) AppealItem {
	if appeal == nil {
		return AppealItem{}
	}
	return AppealItem{
		ID:        appeal.ID,
		EventID:   appeal.EventID,
		UserID:    appeal.UserID,
		Reason:    appeal.Reason,
		Status:    string(appeal.Status),
		Note:      appeal.Note,
		CreatedAt: appeal.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// currentUserID 从鉴权上下文取当前用户。
func currentUserID(c *gin.Context) (uint64, bool) {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil || claims.UserID.Uint64() == 0 {
		res.FailWithMsg("请先登录", c)
		return 0, false
	}
	return claims.UserID.Uint64(), true
}

// parseID 取路径里的数字 ID。
func parseID(c *gin.Context) (uint64, bool) {
	var req struct {
		ID uint64 `uri:"id" binding:"required"`
	}
	if err := c.ShouldBindUri(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return 0, false
	}
	if req.ID == 0 {
		res.FailWithError(apperr.New(apperr.CodeModerationEventNotFound), c)
		return 0, false
	}
	return req.ID, true
}
