package chat_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 低风险：普通用户消息列表的已删过滤还是 NOT IN (subquery)。
// chat_delete_user.go 在依赖状态表子查询排除消息。配合当前索引能跑，但如果以后状态表很大，NOT EXISTS 或显式 LEFT JOIN ... IS NULL 往往更容易拿到稳定执行计划。

func (h ChatApi) ChatSessionDeleteUserView(c *gin.Context) {
	cr := middleware.GetBindJson[ChatSessionDeleteUserRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	if len(cr.SessionIDList) == 0 {
		res.FailWithMsg("请输入要删除的会话 session_id 列表", c)
		return
	}

	var list []models.ChatSessionModel
	if err := h.App.DB.Find(&list, "user_id = ? and session_id IN ?", claims.UserID, cr.SessionIDList).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	if len(list) > 0 {
		err := h.App.DB.Transaction(func(tx *gorm.DB) error {
			// 用户删除整个会话时，不逐条写消息删除状态，
			// 而是把当前会话推进到“清空水位”，避免大会话删除时产生大量写入。
			if err := clearChatSessions(tx, list); err != nil {
				return err
			}
			// 会话本身仍然对当前用户做软删除，方便列表页隐藏。
			return tx.Delete(&list).Error
		})
		if err != nil {
			res.FailWithError(err, c)
			return
		}
	}

	res.OkWithMsg(fmt.Sprintf("请求删除会话%d个，成功%d条", len(cr.SessionIDList), len(list)), c)
}

// clearChatSessions 批量推进会话清空水位。
// 这里不会逐条写消息删除状态，而是把每个会话更新到当前最大的消息 ID，
// 这样后续列表查询只需要按水位过滤即可。
func clearChatSessions(tx *gorm.DB, list []models.ChatSessionModel) error {
	if len(list) == 0 {
		return nil
	}

	sessionIDList := extractSessionIDs(list)
	// 每个会话只需要记住“当前已清空到哪条消息”，
	// 后续列表查询直接按这个水位过滤即可。
	maxMsgIDMap, err := loadSessionMaxMsgIDMap(tx, sessionIDList)
	if err != nil {
		return err
	}

	idList := make([]ctype.ID, 0, len(list))
	caseSQL := "CASE id"
	args := make([]any, 0, len(list)*2)
	for _, session := range list {
		clearBeforeMsgID := session.ClearBeforeMsgID
		if maxMsgIDMap[session.SessionID] > clearBeforeMsgID {
			clearBeforeMsgID = maxMsgIDMap[session.SessionID]
		}
		idList = append(idList, session.ID)
		caseSQL += " WHEN ? THEN ?"
		args = append(args, session.ID, clearBeforeMsgID)
	}
	caseSQL += " ELSE clear_before_msg_id END"

	// 批量更新清空水位，避免按会话逐条 update。
	return tx.Model(&models.ChatSessionModel{}).
		Where("id IN ?", idList).
		Updates(map[string]any{
			"clear_before_msg_id": gorm.Expr(caseSQL, args...),
			"unread_count":        0,
		}).Error
}

// 从会话列表里提取 session_id，供批量查询复用。
func extractSessionIDs(list []models.ChatSessionModel) []string {
	sessionIDList := make([]string, 0, len(list))
	for _, item := range list {
		sessionIDList = append(sessionIDList, item.SessionID)
	}
	return sessionIDList
}

type chatSessionMaxMsgRow struct {
	SessionID string
	MaxMsgID  ctype.ID
}

// loadSessionMaxMsgIDMap 查询每个会话当前最大的消息 ID。
// 返回值用于计算会话删除后的 clear_before_msg_id。
func loadSessionMaxMsgIDMap(tx *gorm.DB, sessionIDList []string) (map[string]ctype.ID, error) {
	if len(sessionIDList) == 0 {
		return map[string]ctype.ID{}, nil
	}

	var rows []chatSessionMaxMsgRow
	err := tx.Model(&models.ChatMsgModel{}).
		Select("session_id, MAX(id) AS max_msg_id").
		Where("session_id IN ?", sessionIDList).
		Group("session_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]ctype.ID, len(rows))
	for _, row := range rows {
		result[row.SessionID] = row.MaxMsgID
	}
	return result, nil
}
