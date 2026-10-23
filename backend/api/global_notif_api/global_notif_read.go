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

func (h GlobalNotifApi) GlobalNotifReadView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	if len(cr.IDList) == 0 {
		res.FailWithMsg("请输入要读取的消息 id 列表", c)
		return
	}

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
	if len(notifList) == 0 {
		res.FailWithMsg("消息不存在", c)
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

			// 已存在未读记录时，只在本次更新真正命中时才累计成功数。
			updateResult := tx.Model(&models.UserGlobalNotifModel{}).
				Where(match).
				Where("deleted_at IS NULL AND is_read = ?", false).
				Updates(map[string]any{
					"is_read":    true,
					"read_at":    &now,
					"updated_at": now,
				})
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected > 0 {
				successCount++
				continue
			}

			userNotif := models.UserGlobalNotifModel{
				MsgID:  notif.ID,
				UserID: claims.UserID,
				IsRead: true,
				ReadAt: &now,
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

	if successCount == 0 {
		res.FailWithMsg("没有可标记已读的消息", c)
		return
	}

	res.OkWithMsg(fmt.Sprintf("标记已读%d条消息", successCount), c)
}
