package chat_api

import (
	"fmt"
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

// ChatMsgReadUserView 批量标记当前用户收到的消息为已读。
// 这里只处理“当前用户是接收方”的消息；自己发送的消息和不存在的消息会被自动忽略。
func (h ChatApi) ChatMsgReadUserView(c *gin.Context) {
	cr := middleware.GetBindJson[ChatMsgReadUserRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	db := h.App.DB
	logger := h.App.Logger

	if len(cr.MsgIDList) == 0 {
		res.FailWithMsg("请输入要标记已读的消息 id 列表", c)
		return
	}

	var msgList []models.ChatMsgModel
	if err := db.Select("id", "session_id", "sender_id", "receiver_id").
		Find(&msgList, "id IN ? AND receiver_id = ? AND msg_status < ?", cr.MsgIDList, claims.UserID, chat_msg_enum.MsgStatusRead).Error; err != nil {
		res.FailWithError(err, c)
		return
	}
	if len(msgList) == 0 {
		res.FailWithMsg("没有可标记已读的消息", c)
		return
	}

	now := time.Now()

	// 从消息列表中提取主键 id，供批量更新消息状态使用
	msgIDList := make([]ctype.ID, 0, len(msgList))

	// 统计本次每个会话实际减少的未读数量
	sessionUnreadDelta := make(map[string]int, len(msgList))

	for _, item := range msgList {
		msgIDList = append(msgIDList, item.ID)
		sessionUnreadDelta[item.SessionID]++
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ChatMsgModel{}).
			Where("id IN ?", msgIDList).
			Updates(map[string]any{
				"msg_status": chat_msg_enum.MsgStatusRead,
				"read_at":    &now,
			}).Error; err != nil {
			return err
		}
		return decreaseChatSessionUnreadCount(tx, claims.UserID, sessionUnreadDelta)
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	// WS 推送已读回执
	pushMap := buildChatMsgReadPushMap(msgList, claims.UserID, now)
	for senderID, pushList := range pushMap {
		for _, push := range pushList {
			successCount := res.SendWsMsg(push, chat_service.GetOnlineUserStore(), senderID)
			if successCount == 0 {
				logger.Infof("聊天已读回执未推送到在线连接: 发送者ID=%d 会话ID=%s", senderID, push.SessionID)
			}
		}
	}

	res.OkWithMsg(fmt.Sprintf("批量标记已读%d条消息", len(msgList)), c)
}

// buildChatMsgReadPushMap 按发送方和会话维度整理已读回执，避免一条消息触发一条 ws 推送。
func buildChatMsgReadPushMap(msgList []models.ChatMsgModel, readerID ctype.ID, readAt time.Time) map[ctype.ID][]ChatMsgReadPush {
	groupMap := make(map[ctype.ID]map[string][]ctype.ID)
	for _, item := range msgList {
		sessionMap, ok := groupMap[item.SenderID]
		if !ok {
			sessionMap = make(map[string][]ctype.ID)
			groupMap[item.SenderID] = sessionMap
		}
		sessionMap[item.SessionID] = append(sessionMap[item.SessionID], item.ID)
	}

	pushMap := make(map[ctype.ID][]ChatMsgReadPush, len(groupMap))
	for senderID, sessionMap := range groupMap {
		pushList := make([]ChatMsgReadPush, 0, len(sessionMap))
		for sessionID, msgIDList := range sessionMap {
			pushList = append(pushList, ChatMsgReadPush{
				MsgType:   chat_msg_enum.MsgTypeRead,
				SessionID: sessionID,
				ReaderID:  readerID,
				MsgIDList: msgIDList,
				ReadAt:    readAt,
			})
		}
		pushMap[senderID] = pushList
	}
	return pushMap
}

// 按本次批量已读命中的消息数量递减会话未读数，保证未读数不会减成负数。
func decreaseChatSessionUnreadCount(tx *gorm.DB, userID ctype.ID, sessionUnreadDelta map[string]int) error {
	if len(sessionUnreadDelta) == 0 {
		return nil
	}

	caseSQL := "CASE session_id"
	args := make([]any, 0, len(sessionUnreadDelta)*3)
	sessionIDList := make([]string, 0, len(sessionUnreadDelta))
	for sessionID, delta := range sessionUnreadDelta {
		caseSQL += " WHEN ? THEN CASE WHEN unread_count >= ? THEN unread_count - ? ELSE 0 END"
		args = append(args, sessionID, delta, delta)
		sessionIDList = append(sessionIDList, sessionID)
	}
	caseSQL += " ELSE unread_count END"

	return tx.Unscoped().Model(&models.ChatSessionModel{}).
		Where("user_id = ? AND session_id IN ?", userID, sessionIDList).
		Update("unread_count", gorm.Expr(caseSQL, args...)).Error
}
