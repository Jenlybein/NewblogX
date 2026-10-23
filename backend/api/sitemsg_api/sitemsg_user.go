package sitemsg_api

import (
	"myblogx/api/global_notif_api"
	"myblogx/common/res"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/message_enum"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h SitemsgApi) SitemsgUserView(c *gin.Context) {
	claims := jwts.MustGetClaimsByGin(c)

	type messageCountRow struct {
		Type  message_enum.Type
		Count int64
	}
	var msgCounts []messageCountRow
	if err := h.App.DB.Model(&models.ArticleMessageModel{}).
		Select("type, COUNT(*) AS count").
		Where("receiver_id = ? AND is_read = ?", claims.UserID, false).
		Group("type").
		Scan(&msgCounts).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	var data SitemsgUserResponse
	for _, item := range msgCounts {
		switch item.Type {
		case message_enum.CommentArticleType, message_enum.CommentReplyType:
			data.CommentMsgCount += int(item.Count)
		case message_enum.DiggArticleType, message_enum.DiggCommentType, message_enum.FavorArticleType:
			data.DiggFavorMsgCount += int(item.Count)
		case message_enum.SystemType:
			data.SystemMsgCount += int(item.Count)
		}
	}

	// 计算未读的私信总数
	if err := h.App.DB.Model(&models.ChatSessionModel{}).
		Where("user_id = ? AND unread_count > 0", claims.UserID).
		Select("COALESCE(SUM(unread_count), 0)").
		Scan(&data.PrivateMsgCount).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	state, err := global_notif_api.LoadUserGlobalNotifState(h.App.DB, claims.UserID, nil)
	if err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}

	globalMsgCount, err := countUnreadGlobalNotif(h.App.DB, state)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	data.GlobalMsgCount = globalMsgCount

	res.OkWithData(data, c)
}

func countUnreadGlobalNotif(db *gorm.DB, state global_notif_api.UserGlobalNotifState) (int, error) {
	readMsgIDList := make([]ctype.ID, 0)
	for msgID, userNotif := range state.UserNotifMap {
		if userNotif.IsRead {
			readMsgIDList = append(readMsgIDList, msgID)
		}
	}

	query := global_notif_api.BuildUserVisibleGlobalNotifListQuery(db, state).Model(&models.GlobalNotifModel{})
	if len(readMsgIDList) > 0 {
		query = query.Where("id NOT IN ?", readMsgIDList)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
