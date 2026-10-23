package chat_repo

import (
	"fmt"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func EnsureSessions(tx *gorm.DB, senderID, receiverID ctype.ID, sessionID string) error {
	userMap, err := read_repo.LoadUserDisplayMap(tx, []ctype.ID{senderID, receiverID})
	if err != nil {
		return err
	}

	sessions := []models.ChatSessionModel{
		{
			SessionID:        sessionID,
			UserID:           senderID,
			ReceiverID:       receiverID,
			ReceiverNickname: userMap[receiverID].Nickname,
			ReceiverAvatar:   userMap[receiverID].Avatar,
		},
	}
	if senderID != receiverID {
		sessions = append(sessions, models.ChatSessionModel{
			SessionID:        sessionID,
			UserID:           receiverID,
			ReceiverID:       senderID,
			ReceiverNickname: userMap[senderID].Nickname,
			ReceiverAvatar:   userMap[senderID].Avatar,
		})
	}

	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "receiver_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"session_id":        sessionID,
			"receiver_nickname": gorm.Expr("excluded.receiver_nickname"),
			"receiver_avatar":   gorm.Expr("excluded.receiver_avatar"),
			"deleted_at":        nil,
			"unread_count":      gorm.Expr("CASE WHEN deleted_at IS NOT NULL THEN 0 ELSE unread_count END"),
		}),
	}).Create(&sessions).Error
}

func UpdateLastMsgSession(tx *gorm.DB, sessionID string, lastMsgID ctype.ID, lastMsgContent string, sendTime time.Time, senderID, receiverID ctype.ID) error {
	expectedRows := int64(2)
	updates := map[string]any{
		"last_msg_id":      lastMsgID,
		"last_msg_content": lastMsgContent,
		"last_msg_time":    sendTime,
		"unread_count": gorm.Expr(
			"CASE WHEN user_id = ? AND receiver_id = ? THEN unread_count + 1 ELSE unread_count END",
			receiverID,
			senderID,
		),
	}
	if senderID == receiverID {
		expectedRows = 1
		updates["unread_count"] = gorm.Expr("unread_count")
	}
	result := tx.Model(&models.ChatSessionModel{}).
		Where(
			`session_id = ? AND (
				(user_id = ? AND receiver_id = ?) OR
				(user_id = ? AND receiver_id = ?)
			)`,
			sessionID,
			senderID, receiverID,
			receiverID, senderID,
		).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != expectedRows {
		return fmt.Errorf("会话更新数量异常: session_id=%s affected=%d", sessionID, result.RowsAffected)
	}

	return nil
}

func CreateMessage(tx *gorm.DB, msg *models.ChatMsgModel) error {
	return tx.Create(msg).Error
}
