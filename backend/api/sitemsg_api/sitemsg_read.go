package sitemsg_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum/message_enum"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *SitemsgApi) SitemsgReadView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[SitemsgReadRequest](c)

	claims := jwts.MustGetClaimsByGin(c)

	if cr.ID == 0 && cr.T == 0 {
		res.FailWithMsg("id 和 t 不能同时为空", c)
		return
	}

	if cr.ID != 0 {
		var msg models.ArticleMessageModel
		if err := app.DB.Take(&msg, "id = ? and receiver_id = ?", cr.ID, claims.UserID).Error; err != nil {
			res.FailWithMsg("消息不存在", c)
			return
		}

		if msg.IsRead {
			res.FailWithMsg("消息已被读取", c)
			return
		}

		now := time.Now()
		if err := app.DB.Model(&msg).Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		}).Error; err != nil {
			res.FailWithError(err, c)
			return
		}

		res.OkWithMsg("标记已读成功", c)
		return
	}

	var typeList []message_enum.Type
	switch cr.T {
	case 1:
		typeList = append(typeList, message_enum.CommentArticleType, message_enum.CommentReplyType)
	case 2:
		typeList = append(typeList, message_enum.DiggArticleType, message_enum.DiggCommentType, message_enum.FavorArticleType)
	case 3:
		typeList = append(typeList, message_enum.SystemType)
	}

	var msgList []models.ArticleMessageModel
	if err := app.DB.Find(&msgList, "receiver_id = ? and type in ? and is_read = ?", claims.UserID, typeList, false).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	if len(msgList) > 0 {
		now := time.Now()
		if err := app.DB.Model(&msgList).Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		}).Error; err != nil {
			res.FailWithError(err, c)
			return
		}
	}

	res.OkWithMsg(fmt.Sprintf("批量标记已读%d条消息", len(msgList)), c)
}
