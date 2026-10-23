package global_notif_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h GlobalNotifApi) GlobalNotifAdminRemoveView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[models.IDListRequest](c)

	if len(cr.IDList) == 0 {
		res.FailWithMsg("请输入要删除的公告 id 列表", c)
		return
	}

	var list []models.GlobalNotifModel
	if err := app.DB.Find(&list, "id IN ?", cr.IDList).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	if len(list) > 0 {
		if err := app.DB.Delete(&list).Error; err != nil {
			res.FailWithError(err, c)
			return
		}
	} else {
		res.FailWithMsg("未找到需要删除的公告", c)
		return
	}
	res.OkWithMsg(fmt.Sprintf("请求删除公告%d个，成功%d条", len(cr.IDList), len(list)), c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName:  "global_notif_admin_remove",
		TargetType:  "global_notif",
		Success:     true,
		Message:     fmt.Sprintf("请求删除公告%d个，成功%d条", len(cr.IDList), len(list)),
		RequestBody: map[string]any{"id_list": cr.IDList},
		ResponseBody: map[string]any{
			"deleted_count": len(list),
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}

func (h GlobalNotifApi) GlobalNotifUserRemoveView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	state, err := LoadUserGlobalNotifState(h.App.DB, claims.UserID, nil)
	if err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}

	var notifList []models.GlobalNotifModel
	if err := BuildUserVisibleGlobalNotifListQuery(h.App.DB, state).Where("id IN ?", cr.IDList).Find(&notifList).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	// 用户只能删除自己当前“本来就看得见”的通知。
	// 如果传入的 ID 不可见、已过期或不存在，这里会被自然过滤掉。
	if len(notifList) == 0 {
		res.OkWithMsg(fmt.Sprintf("请求删除公告%d个，成功0条", len(cr.IDList)), c)
		return
	}

	var successCount int
	err = h.App.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, notif := range notifList {
			match := map[string]any{
				"msg_id":  notif.ID,
				"user_id": claims.UserID,
			}

			// 先尝试软删当前活记录，只有真正删掉时才计入成功数。
			updateResult := tx.Unscoped().Model(&models.UserGlobalNotifModel{}).
				Where(match).
				Where("deleted_at IS NULL").
				Updates(map[string]any{
					"deleted_at": now,
					"updated_at": now,
				})
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected > 0 {
				successCount++
				continue
			}

			// 如果用户此前从未产生过这条通知的个人态记录，
			// 直接创建一条带 deleted_at 的墓碑记录即可。
			// 这样后续列表查询时，仍然能识别“这条通知用户已经删过”。
			userNotif := models.UserGlobalNotifModel{
				Model: models.Model{
					DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
				},
				MsgID:  notif.ID,
				UserID: claims.UserID,
			}
			createResult := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "msg_id"},
					{Name: "user_id"},
				},
				DoNothing: true,
			}).Create(&userNotif)
			if createResult.Error != nil {
				return createResult.Error
			}
			if createResult.RowsAffected > 0 {
				successCount++
			}
		}
		return nil
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithMsg(fmt.Sprintf("请求删除公告%d个，成功%d条", len(cr.IDList), successCount), c)
}
