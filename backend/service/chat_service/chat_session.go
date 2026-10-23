package chat_service

import (
	"fmt"
	"time"

	"myblogx/models/ctype"
	"myblogx/repository/chat_repo"

	"gorm.io/gorm"
)

// 为一对聊天用户生成稳定的逻辑会话标识。
func buildSessionID(a, b ctype.ID) string {
	if a < b {
		return fmt.Sprintf("chat:%d:%d", a, b)
	}
	return fmt.Sprintf("chat:%d:%d", b, a)
}

// 检查聊天双方的会话记录，不存在则分别创建。
func ensureChatSessions(tx *gorm.DB, req ToChatRequest, sessionID string) error {
	return chat_repo.EnsureSessions(tx, req.SenderID, req.ReceiverID, sessionID)
}

// updateLastMsgSession 更新双方会话的最后一条消息。
// 发送方只更新摘要，接收方同时累加未读数。
func updateLastMsgSession(tx *gorm.DB, sessionID string, lastMsgID ctype.ID, lastMsgContent string, sendTime time.Time, senderID, receiverID ctype.ID) error {
	return chat_repo.UpdateLastMsgSession(tx, sessionID, lastMsgID, lastMsgContent, sendTime, senderID, receiverID)
}
