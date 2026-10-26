package http

import (
	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/page"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

type UserLoginListRequest struct {
	page.PageInfo
	UserID  ctype.ID `form:"user_id"`
	IP      string   `form:"ip"`
	StartAt string   `form:"start_at"`
	EndAt   string   `form:"end_at"`
	Type    int8     `form:"type" binding:"required,oneof=1 2" enums:"1,2"` // 1：用户查自己 2：管理员查任意用户
}

// UserLoginLogList 查询用户登录日志（管理员）。
// @Summary 用户登录日志
// @Tags user
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param user_id query string false "用户 ID"
// @Param ip query string false "登录 IP"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param type query int true "查询类型：1 用户查自己，2 管理员查任意用户"
// @Success 200 {object} kernel.Envelope[kernel.ListData[clickhouse.LoginEventRecord]]
// @Router /users/login/log [get]
func (h *Handler) UserLoginLogList(c *gin.Context) {
	cr := middleware.GetBindQuery[UserLoginListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	if !claims.IsAdmin() {
		res.FailWithMsg("普通角色不可访问日志接口", c)
		return
	}

	if cr.Type == 1 {
		if cr.UserID.IsZero() {
			cr.UserID = claims.UserID
		}
		if claims.UserID != cr.UserID {
			res.FailWithMsg("非管理员用户不能查询其他用户登录日志", c)
			return
		}
	}
	if cr.Type == 2 && !claims.IsAdmin() {
		res.FailWithMsg("非管理员用户不能查询其他用户登录日志", c)
		return
	}

	eventName := "login_success"
	success := true
	list, count, err := clickhouse.ListLoginEvents(h.logs, clickhouse.LoginEventQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		UserID:    cr.UserID,
		IP:        cr.IP,
		EventName: eventName,
		Success:   &success,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithList(list, int(count), c)
}
