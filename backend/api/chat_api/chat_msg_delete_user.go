package chat_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 低风险：普通用户消息列表的已删过滤还是 NOT IN (subquery)。
// chat_delete_user.go 在依赖状态表子查询排除消息。配合当前索引能跑，但如果以后状态表很大，NOT EXISTS 或显式 LEFT JOIN ... IS NULL 往往更容易拿到稳定执行计划。

func (h ChatApi) ChatMsgDeleteUserView(c *gin.Context) {
	cr := middleware.GetBindJson[ChatMsgDeleteUserRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	if len(cr.MsgIDList) == 0 {
		res.FailWithMsg("请输入要删除的消息 id 列表", c)
		return
	}

	var msgList []models.ChatMsgModel
	if err := h.App.DB.Select("id", "session_id").
		Find(&msgList, "id IN ? AND (sender_id = ? OR receiver_id = ?)", cr.MsgIDList, claims.UserID, claims.UserID).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	if len(msgList) > 0 {
		// 单条消息删除保留“用户消息状态表”，只影响当前用户视图，不动原消息数据。
		if err := insertChatMsgUserStates(h.App.DB, claims.UserID, msgList); err != nil {
			res.FailWithError(err, c)
			return
		}
	}

	res.OkWithMsg(fmt.Sprintf("请求删除消息%d个，成功%d条", len(cr.MsgIDList), len(msgList)), c)
}

// buildChatMsgVisibleWhere 构造消息列表的可见性过滤条件。
// 普通用户需要同时排除：
// 1. 会话清空水位之前的旧消息
// 2. 当前用户单独删除过的消息
// 管理员查看时不做用户侧删除过滤，只保留会话范围条件。
func buildChatMsgVisibleWhere(db *gorm.DB, userID ctype.ID, sessionID string, clearBeforeMsgID ctype.ID, allowUnscoped bool) *gorm.DB {
	query := db
	if clearBeforeMsgID > 0 {
		// 会话清空后，只返回水位之后的新消息。
		query = query.Where("id > ?", clearBeforeMsgID)
	}
	if allowUnscoped {
		return query
	}
	// 普通用户模式下，需要排除当前用户单独删除过的消息。
	subQuery := db.Unscoped().Model(&models.ChatMsgUserStateModel{}).
		Select("msg_id").
		Where("user_id = ? AND session_id = ? AND deleted_at IS NOT NULL", userID, sessionID)
	return query.Not("id IN (?)", subQuery)
}

// insertChatMsgUserStates 批量写入“用户删除消息”状态。
// 如果状态已存在，则覆盖删除时间，保证重复删除请求幂等。
func insertChatMsgUserStates(tx *gorm.DB, userID ctype.ID, msgList []models.ChatMsgModel) error {
	if len(msgList) == 0 {
		return nil
	}

	now := time.Now()
	// 这里的软删时间表示“该用户何时删除了这条消息”，
	// 管理员查看消息列表时会优先展示这份删除时间。
	stateList := make([]models.ChatMsgUserStateModel, 0, len(msgList))
	for _, msg := range msgList {
		stateList = append(stateList, models.ChatMsgUserStateModel{
			Model: models.Model{
				DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
			},
			MsgID:     msg.ID,
			UserID:    userID,
			SessionID: msg.SessionID,
		})
	}

	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "msg_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"session_id", "deleted_at", "updated_at"}),
	}).Create(&stateList).Error
}
