// Package mysql 是私信模块的 MySQL 适配器。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「会话可见集 / 会话未读数 / 清空界线」的唯一实现处。
//
// 消息列表、整会话已读与 30 分钟并账都从这里拿同一个可见性口径：
// 会话内、严格晚于清空时间水位、且未被当前用户级软删的消息。
// 会话未读数再叠加「接收者是我且未读」这层边界。
//
// clear_before_msg_id 存的是「清到哪条消息」的主键，但主键不是时序：客户端幂等键
// 派生的消息 ID 是 [2^62,2^63) 区间的 FNV 哈希，拿 id > 水位做过滤会随机
// 放过/挡掉历史消息。真相是那行消息的 send_time，所以可见性条件是
//
//	send_time > (SELECT send_time FROM chat_msg_models WHERE id = 水位)
//
// 子查询必须 Unscoped：被清空的那条消息可能之后被用户删掉（软删），
// 默认范围会把水位行过滤掉，子查询返回 NULL，整段历史就又冒出来了。
//
// 消息中心角标读的是会话行上的派生列 unread_count；它由发送 / 已读的增量写入，
// 并由 30 分钟并账按这里同一份口径覆盖写回，所以四者不会各说各话。
// 零 DDL：只复用 clear_before_msg_id / unread_count 两列，没有读位置列。

// visibilityScope 是一份可见性口径。四个用例只消费它，不再各自拼 SQL。
//
//	VisibleMessages：会话可见集（含自己发的消息）；
//	UnreadMessages ：在可见集上叠加「接收者是我且未读」。
type visibilityScope struct {
	ctx         context.Context
	db          *gorm.DB
	sessionID   string
	userID      ctype.ID
	clearBefore ctype.ID
}

// newVisibilityScope 从会话行取「我」自己的清空水位，构造唯一口径。
func (r *Repo) newVisibilityScope(ctx context.Context, sessionID string, userID ctype.ID) (visibilityScope, error) {
	var session sessionRow
	if err := r.db.WithContext(ctx).Model(&sessionRow{}).
		Select("clear_before_msg_id").
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Take(&session).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return visibilityScope{}, err
	}
	return visibilityScope{
		ctx:         ctx,
		db:          r.db,
		sessionID:   sessionID,
		userID:      userID,
		clearBefore: ctype.NewID(session.ClearBeforeMsgID.Uint64()),
	}, nil
}

// withClearBefore 覆盖水位（消息列表用例层已从会话行读出并传入）。
func (v visibilityScope) withClearBefore(clearBefore ctype.ID) visibilityScope {
	v.clearBefore = clearBefore
	return v
}

// clauses 是唯一一份可见性条件：会话 + 清空时间水位 + 用户级软删。
// 水位为 0 时不加过滤；unscoped=true 时连软删行一起看（管理员视角）。
func (v visibilityScope) clauses(tx *gorm.DB, unscoped bool) *gorm.DB {
	if unscoped {
		tx = tx.Unscoped()
	}
	tx = v.applyClearWatermark(tx)
	if !unscoped {
		tx = tx.Where("id NOT IN (?)", v.userDeletedMsgIDs())
	}
	return tx
}

// visible 是消息列表口径：含自己发的消息，只过滤会话 / 水位 / 用户级删除。
func (v visibilityScope) visible(unscoped bool) *gorm.DB {
	tx := v.db.WithContext(v.ctx).Model(&msgRow{}).Where("session_id = ?", v.sessionID)
	return v.clauses(tx, unscoped)
}

// unread 是未读子集：可见集 + 接收者是我且未读。
func (v visibilityScope) unread() *gorm.DB {
	return v.clauses(
		v.db.WithContext(v.ctx).Model(&msgRow{}).
			Where("session_id = ? AND receiver_id = ? AND msg_status < ?",
				v.sessionID, v.userID, int8(domain.StatusRead)),
		false,
	)
}

// applyClearWatermark 给"可见消息"查询加上时间水位过滤；水位为零时不加。
func (v visibilityScope) applyClearWatermark(tx *gorm.DB) *gorm.DB {
	if v.clearBefore.IsZero() {
		return tx
	}
	return tx.Where("send_time > (?)", v.clearWatermarkSubquery())
}

// clearWatermarkSubquery 由清空水位 ID 反查被清空那行的 send_time（Unscoped）。
func (v visibilityScope) clearWatermarkSubquery() *gorm.DB {
	return v.db.WithContext(v.ctx).Unscoped().Model(&msgRow{}).
		Select("send_time").
		Where("id = ?", v.clearBefore)
}

// userDeletedMsgIDs 是"该用户在该会话里已删除"的消息 ID 子查询（Unscoped）。
func (v visibilityScope) userDeletedMsgIDs() *gorm.DB {
	return v.db.WithContext(v.ctx).Unscoped().Model(&stateRow{}).
		Select("msg_id").
		Where("user_id = ? AND session_id = ? AND deleted_at IS NOT NULL", v.userID, v.sessionID)
}

// latestMessage 是一条会话里"时间最晚"的消息。
//
// 主键不是时序：客户端幂等键派生的 ID 落在 [2^62,2^63) 的 FNV 哈希区间，
// MAX(id) 会挑到一条时间上更早的消息，清空后旧历史重现、未读被并账复活。
// 同毫秒并列时用 id 做确定性的最终裁决（只定序，不当时序键）。
type latestMessage struct {
	ID       ctype.ID
	SendTime time.Time
}

// after 判断候选消息是否比现有水位更晚。
//
// 现有水位存的是"清到哪条消息"的主键，换算成那条消息的 send_time 才是时间水位；
// 水位为 0 / 水位行缺失时没有过滤，任何可见候选都比它晚。
func (m latestMessage) after(watermarkTimes map[uint64]time.Time, clearBefore ctype.ID) bool {
	if clearBefore.IsZero() {
		return true
	}
	current, ok := watermarkTimes[clearBefore.Uint64()]
	if !ok {
		return true
	}
	return m.SendTime.After(current) || (m.SendTime.Equal(current) && m.ID.Uint64() > clearBefore.Uint64())
}

// loadLatestMessages 按 send_time 取每条会话时间最晚的一行。
//
// 用相关子查询而不是 GROUP BY + MAX(send_time) 再回连：后者遇到同毫秒并列会
// 返回多行，水位就变成不确定的。这里 ORDER BY send_time DESC, id DESC 只取一行。
func loadLatestMessages(tx *gorm.DB, sessionIDs []string) (map[string]latestMessage, error) {
	var rows []struct {
		SessionID string
		ID        ctype.ID
		SendTime  time.Time
	}
	if err := tx.Table("chat_msg_models AS m").
		Select("m.session_id, m.id, m.send_time").
		Where("m.session_id IN ? AND m.id = (?)", sessionIDs,
			tx.Table("chat_msg_models AS latest").
				Select("latest.id").
				Where("latest.session_id = m.session_id").
				Order("latest.send_time DESC, latest.id DESC").
				Limit(1),
		).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]latestMessage, len(rows))
	for _, row := range rows {
		out[row.SessionID] = latestMessage{ID: ctype.NewID(row.ID.Uint64()), SendTime: row.SendTime}
	}
	return out, nil
}

// loadWatermarkSendTimes 把会话行里的清空水位 ID 换算成 send_time。
//
// 子查询必须 Unscoped：水位行可能已被用户级软删，否则整段历史会重新冒出来。
func loadWatermarkSendTimes(tx *gorm.DB, sessions []sessionRow) (map[uint64]time.Time, error) {
	ids := make([]uint64, 0, len(sessions))
	for _, row := range sessions {
		if !row.ClearBeforeMsgID.IsZero() {
			ids = append(ids, row.ClearBeforeMsgID.Uint64())
		}
	}
	if len(ids) == 0 {
		return map[uint64]time.Time{}, nil
	}
	var rows []struct {
		ID       ctype.ID
		SendTime time.Time
	}
	if err := tx.Unscoped().Table("chat_msg_models").
		Select("id, send_time").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]time.Time, len(rows))
	for _, row := range rows {
		out[row.ID.Uint64()] = row.SendTime
	}
	return out, nil
}
