package global_notif_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum/global_notif_enum"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
)

func (h GlobalNotifApi) GlobalNotifCreateView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[GlobalNotifCreateRequest](c)

	claims := jwts.MustGetClaimsByGin(c)
	if claims.IsAdmin() == false {
		res.FailWithMsg("权限错误", c)
		return
	}

	// 默认过期时间
	now := time.Now()
	if cr.ExpireTime != nil {
		if cr.ExpireTime.Before(now.Add(23 * time.Hour)) {
			res.FailWithMsg("过期时间不能小于一天", c)
			return
		}
	} else {
		// 默认过期时间为一周
		nextWeek := now.Add(7 * 24 * time.Hour)
		cr.ExpireTime = &nextWeek
	}

	// 默认可视规则
	if cr.UserVisibleRule == 0 {
		cr.UserVisibleRule = global_notif_enum.UserVisibleRegisteredUsers
	}

	// 检测是否有重复
	var model models.GlobalNotifModel
	if err := app.DB.Take(&model, "title = ?", cr.Title).Error; err == nil {
		res.FailWithMsg("全局通知标题重复", c)
		return
	}

	// 执行创建
	model = models.GlobalNotifModel{
		ActionUser:      claims.UserID,
		ExpireTime:      *cr.ExpireTime,
		UserVisibleRule: cr.UserVisibleRule,
		Title:           cr.Title,
		Content:         cr.Content,
		Href:            cr.Href,
		Icon:            cr.Icon,
	}
	if err := app.DB.Create(&model).Error; err != nil {
		res.FailWithMsg(fmt.Sprintf("全局通知创建失败 %v", err), c)
		return
	}
	res.OkWithData(GlobalNotifCreateResponse{
		ID:              model.ID,
		ExpireTime:      model.ExpireTime,
		UserVisibleRule: model.UserVisibleRule,
		Title:           model.Title,
		Content:         model.Content,
		Icon:            model.Icon,
		Href:            model.Href,
	}, c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName:        "global_notif_create",
		TargetType:        "global_notif",
		TargetID:          model.ID.String(),
		Success:           true,
		Message:           "创建全局通知成功",
		RequestBody:       cr,
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}
