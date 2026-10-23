package chat_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/chat_msg_enum"
	"myblogx/service/chat_service"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ChatSessionListView 返回当前登录用户的会话列表。
func (h *ChatApi) ChatSessionListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ChatSessionListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	switch cr.Type {
	case 1:
		cr.UserID = claims.UserID
	case 2:
		if !claims.IsAdmin() {
			res.FailWithMsg("权限不足", c)
			return
		}
		if cr.UserID == 0 {
			res.FailWithMsg("user_id 不能为 0", c)
			return
		}
	}

	queryService := chat_service.NewQueryService(h.App.DB)
	list, count, err := queryService.ListSessions(chat_service.SessionListQuery{
		PageInfo: cr.PageInfo,
		UserID:   cr.UserID,
		Type:     cr.Type,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, count, c)
}

// ChatMsgListView 返回当前登录用户在某个会话下的消息列表。
func (h *ChatApi) ChatMsgListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ChatMsgListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	allowUnscoped := false
	switch cr.Type {
	case 1:
		cr.UserID = claims.UserID
	case 2:
		if !claims.IsAdmin() {
			res.FailWithMsg("权限不足", c)
			return
		}
		if cr.UserID == 0 {
			res.FailWithMsg("user_id 不能为 0", c)
			return
		}
		allowUnscoped = true
	}

	var session models.ChatSessionModel
	sessionQuery := h.App.DB.Select("session_id", "clear_before_msg_id")
	if allowUnscoped {
		sessionQuery = sessionQuery.Unscoped()
	}
	if err := sessionQuery.
		Take(&session, "session_id = ? and user_id = ?", cr.SessionID, cr.UserID).Error; err != nil {
		res.FailWithMsg("会话不存在", c)
		return
	}

	list, count, err := common.ListQuery(models.ChatMsgModel{
		SessionID: cr.SessionID,
	}, common.Options{
		DB:           h.App.DB,
		PageInfo:     cr.PageInfo,
		DefaultOrder: "send_time desc",
		Unscoped:     allowUnscoped,
		Where:        buildChatMsgVisibleWhere(h.App.DB, cr.UserID, cr.SessionID, session.ClearBeforeMsgID, allowUnscoped),
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	stateMap, err := loadChatMsgDeletedAtMap(h.App.DB, cr.UserID, cr.SessionID, allowUnscoped, list)
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	respList := make([]ChatMsgResponse, 0, len(list))
	for _, item := range list {
		data := ChatMsgResponse{
			ID:         item.ID,
			SenderID:   item.SenderID,
			ReceiverID: item.ReceiverID,
			SessionID:  item.SessionID,
			Content:    item.Content,
			SendTime:   item.SendTime,
			MsgStatus:  item.MsgStatus,
			MsgType:    item.MsgType,
			IsSelf:     item.SenderID == cr.UserID,
			IsRead:     int8(item.MsgStatus) >= int8(chat_msg_enum.MsgStatusRead),
		}
		if deletedAt, ok := stateMap[item.ID]; ok {
			data.DeletedAt = &deletedAt
		}
		respList = append(respList, data)
	}

	res.OkWithList(respList, count, c)
}

func loadChatMsgDeletedAtMap(db *gorm.DB, userID ctype.ID, sessionID string, allowUnscoped bool, msgList []models.ChatMsgModel) (map[ctype.ID]time.Time, error) {
	if !allowUnscoped || len(msgList) == 0 {
		return nil, nil
	}

	msgIDList := make([]ctype.ID, 0, len(msgList))
	for _, item := range msgList {
		msgIDList = append(msgIDList, item.ID)
	}

	var stateList []models.ChatMsgUserStateModel
	err := db.Unscoped().
		Find(&stateList, "user_id = ? AND session_id = ? AND msg_id IN ? AND deleted_at IS NOT NULL", userID, sessionID, msgIDList).Error
	if err != nil {
		return nil, err
	}

	stateMap := make(map[ctype.ID]time.Time, len(stateList))
	for _, item := range stateList {
		if item.DeletedAt.Valid {
			stateMap[item.MsgID] = item.DeletedAt.Time
		}
	}
	return stateMap, nil
}
