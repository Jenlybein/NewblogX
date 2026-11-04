package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
)

// 本文件承担「把私信未读重算回会话行」的写入。
//
// 与文章计数并账（ReconcileArticleCount）同一条思路：unread_count 不是账本本身，
// 而是**可以从消息行推导出来的派生值**。发送 / 已读仍然走增量（要即时性），
// 但增量在多端已读、并发发送与已读交错时会留下永久漂移：徽标显示 3、点进去
// 只有 1 条。定期重算覆盖就能自愈，不需要给 session 表加"读位置"列（零 DDL）。
//
// 真值口径（与 ListMessages 的可见性条件保持一致）：
//
//	unread = COUNT(chat_msg_models
//	                WHERE session_id = 该会话
//	                  AND receiver_id = 我
//	                  AND msg_status < 已读
//	                  AND id > clear_before_msg_id)
//
// 四条边界：
//   - 只数「发给我的」：receiver_id = user_id，发送方那侧永远不进未读；
//   - 只数未读：msg_status < StatusRead（已读状态在消息行上，是全局的，
//     1:1 会话里只有接收方会标已读，所以不需要逐用户读位置）；
//   - 尊重清空水位：clear_before_msg_id 之前（含）的消息对该用户不可见，
//     "清除会话"之后重算不能让未读复活；
//   - 用户级删除（chat_msg_user_state_models）也会让消息对自己不可见，
//     重算时同样排除，否则删掉的消息还会占着未读数。

// ReconcileChatUnread 重算并覆盖某个用户某条会话的未读计数，返回 hit=false
// 表示会话行不存在或已软删（此时不写、也不报错）。
//
// 导出供 cron 任务调用；它是**覆盖写**而不是 `unread_count -= n`，所以历史漂移
// 在本轮自愈。与 ADR-0025 的差别只在于触发方式：文章计数由"哪篇文章有 Redis
// 增量"驱动，私信没有 Redis 增量源，改为每轮扫描全部活跃会话（量级是用户会话数，
// 不是消息行数）。
func ReconcileChatUnread(ctx context.Context, db *gorm.DB, sessionID string, userID uint64) (bool, error) {
	if db == nil {
		return false, nil
	}

	var session sessionRow
	if err := db.WithContext(ctx).
		Select("id", "user_id", "receiver_id", "clear_before_msg_id").
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}

	// 软删的会话由 First 的默认范围挡掉（deleted_at IS NULL）；能查到就是活跃会话。
	// 但并账期间正好被软删也不该把未读写回去：UPDATE 仍带默认范围，见下面。
	var unread int64
	// 用户级删除是**软删**：stateRow 带 gorm.DeletedAt，GORM 会自动给子查询补
	// `deleted_at IS NULL`，和这里要求的"已删除"正好相反。必须 Unscoped 才能查到
	// 那些残行 —— 否则子查询恒为空，删掉的消息还会一直占着未读数。
	deletedMsgIDs := db.WithContext(ctx).Unscoped().Model(&stateRow{}).
		Select("msg_id").
		Where("user_id = ? AND session_id = ? AND deleted_at IS NOT NULL", userID, sessionID)
	q := db.WithContext(ctx).Model(&msgRow{}).
		Where("session_id = ? AND receiver_id = ? AND msg_status < ?",
			sessionID, userID, int8(domain.StatusRead)).
		Where("id > ?", session.ClearBeforeMsgID.Uint64()).
		Where("id NOT IN (?)", deletedMsgIDs)
	if err := q.Count(&unread).Error; err != nil {
		return false, err
	}

	res := db.WithContext(ctx).Model(&sessionRow{}).
		Where("id = ? AND deleted_at IS NULL", session.ID.Uint64()).
		UpdateColumn("unread_count", int(unread))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ReconcileAllChatUnread 扫描全部活跃会话并逐条重算，返回实际写回的条数。
//
// 用游标分页而不是一次取全表：会话表会随用户数增长，一次 Load 全表既吃内存
// 也拉长事务。这里不显式开事务 —— 每条会话的覆盖写是独立的，中途失败时
// 已修好的会话保持正确，剩下的下一轮再修（可重入）。
//
// 这也是为什么它挂在既有 30 分钟 tick 上而不是新开调度器：与文章计数并账
// 同一个节奏，不新增定时任务（ADR-0025 的约束）。
func ReconcileAllChatUnread(ctx context.Context, db *gorm.DB, batch int) (int, error) {
	if db == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = 200
	}

	total := 0
	var after uint64
	for {
		var sessions []sessionRow
		if err := db.WithContext(ctx).
			Select("id", "session_id", "user_id").
			Where("id > ? AND deleted_at IS NULL", after).
			Order("id ASC").
			Limit(batch).
			Find(&sessions).Error; err != nil {
			return total, err
		}
		if len(sessions) == 0 {
			return total, nil
		}
		for _, s := range sessions {
			hit, err := ReconcileChatUnread(ctx, db, s.SessionID, s.UserID.Uint64())
			if err != nil {
				return total, err
			}
			if hit {
				total++
			}
			after = s.ID.Uint64()
		}
		if len(sessions) < batch {
			return total, nil
		}
	}
}
