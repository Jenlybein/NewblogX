package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
)

// 本文件承担「把私信未读重算回会话行」的写入。
//
// 与文章计数并账（ReconcileArticleCount）同一条思路：unread_count 不是账本本身，
// 而是**可以从消息行推导出来的派生值**。发送 / 已读仍然走增量（要即时性），
// 但增量在多端已读、并发发送与已读交错时会留下永久漂移：徽标显示 3、点进去
// 只有 1 条。定期重算覆盖就能自愈，不需要给 session 表加"读位置"列（零 DDL）。
//
// 真值口径不在这里另写一份：见 visibility.go 的 visibilityScope.unread()。
//
// 四条边界（消息列表 / 整会话已读 / 消息中心角标 / 本并账共用）：
//   - 会话可见集含自己发的消息；
//   - 未读只数「接收者是我且未读」；
//   - 清空界线按水位行的 send_time 推进，不用最大 ID 挑水位行；
//   - 用户级删除的消息对自己不可见，重算时排除。
//
// 消息中心角标读的是会话行上的派生列 unread_count，由发送 / 已读增量写、
// 由本并账按同一口径覆盖写回，所以四者说法一致（零 DDL，无读位置列）。

// ReconcileChatUnread 重算并覆盖某个用户某条会话的未读计数，返回 hit=false
// 表示会话行不存在或已软删（此时不写、也不报错）。
//
// 导出供 cron 任务调用；它是**覆盖写**而不是 `unread_count -= n`，所以历史漂移
// 在本轮自愈。与 ADR-0016 的差别只在于触发方式：文章计数由"哪篇文章有 Redis
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
	// 与消息列表 / 整会话已读消费同一份可见性口径：并账不再自己拼 SQL。
	scope := visibilityScope{
		ctx:         ctx,
		db:          db,
		sessionID:   sessionID,
		userID:      ctype.NewID(session.UserID.Uint64()),
		clearBefore: ctype.NewID(session.ClearBeforeMsgID.Uint64()),
	}
	var unread int64
	if err := scope.unread().Count(&unread).Error; err != nil {
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
// 游标是 **session_id（字符串），不是主键 id**：主键是雪花号，它的大小既不
// 表示时间先后、也不表示会话顺序，拿它分页等于让"排序 / 水位 / 分页"依赖
// ID 大小（#279 AC4 明令禁止）。session_id 上有 idx_chat_session_id 索引，
// ORDER BY session_id 走索引，正好够用，不需要新增 DDL。
//
// 注意 session_id 不是唯一键：同一条会话在表里有两行（双方各一行）。
// 所以分页按 session_id **去重**取一批会话号，再回表取这批会话的全部行，
// 逐行并账。若按"行"分页并在批次边界把它切成两半，`session_id > after`
// 会把同一会话的另一行漏掉 —— 那个用户的角度就永远不被重算。
//
// 这也是为什么它挂在既有 30 分钟 tick 上而不是新开调度器：与文章计数并账
// 同一个节奏，不新增定时任务（ADR-0016 的约束）。
func ReconcileAllChatUnread(ctx context.Context, db *gorm.DB, batch int) (int, error) {
	if db == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = 200
	}

	total := 0
	var after string
	for {
		var page []sessionRow
		if err := db.WithContext(ctx).
			Model(&sessionRow{}).
			Select("session_id").
			Distinct().
			Where("session_id > ? AND deleted_at IS NULL", after).
			Order("session_id ASC").
			Limit(batch).
			Find(&page).Error; err != nil {
			return total, err
		}
		if len(page) == 0 {
			return total, nil
		}
		sessionIDs := make([]string, 0, len(page))
		for _, s := range page {
			sessionIDs = append(sessionIDs, s.SessionID)
		}
		// 回表取这批会话的全部行：同一条会话的两行（双方各一行）都要并账。
		var rows []sessionRow
		if err := db.WithContext(ctx).
			Select("session_id", "user_id").
			Where("session_id IN ? AND deleted_at IS NULL", sessionIDs).
			Find(&rows).Error; err != nil {
			return total, err
		}
		for _, s := range rows {
			hit, err := ReconcileChatUnread(ctx, db, s.SessionID, s.UserID.Uint64())
			if err != nil {
				return total, err
			}
			if hit {
				total++
			}
		}
		after = page[len(page)-1].SessionID
		if len(page) < batch {
			return total, nil
		}
	}
}
