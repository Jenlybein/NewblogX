package log_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/log_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

type UserLoginListRequest struct {
	common.PageInfo
	UserID  ctype.ID `form:"user_id"`
	IP      string   `form:"ip"`
	StartAt string   `form:"start_at"`
	EndAt   string   `form:"end_at"`
	Type    int8     `form:"type" binding:"required,oneof=1 2"` // 1：用户查自己 2：管理员查任意用户
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
// @Param user_id query int false "用户 ID"
// @Param ip query string false "登录 IP"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param type query int true "查询类型：1 用户查自己，2 管理员查任意用户"
// @Success 200 {object} object{code=int,data=object{list=[]log_service.LoginEventRecord,count=int},msg=string}
// @Router /users/login/log [get]
func (h LogApi) UserLoginLogList(c *gin.Context) {
	cr := middleware.GetBindQuery[UserLoginListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	if claims.Role != enum.RoleAdmin {
		res.FailWithMsg("普通角色不可访问日志接口", c)
		return
	}

	if cr.Type == 1 {
		if cr.UserID == 0 {
			cr.UserID = claims.UserID
		}
		if claims.UserID != cr.UserID {
			res.FailWithMsg("非管理员用户不能查询其他用户登录日志", c)
			return
		}
	}
	if cr.Type == 2 && claims.Role != enum.RoleAdmin {
		res.FailWithMsg("非管理员用户不能查询其他用户登录日志", c)
		return
	}

	eventName := "login_success"
	success := true
	list, count, err := log_service.ListLoginEvents(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), log_service.LoginEventQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: log_service.LogTimeRange{
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
