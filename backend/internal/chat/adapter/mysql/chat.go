// Package mysql 是私信模块的 MySQL 适配器。
package mysql

import (
	"context"
	"fmt"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// sessionRow 对应 chat_session_models。
type sessionRow struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	SessionID        string
	UserID           uint64
	ReceiverID       uint64
	ReceiverNickname string
	ReceiverAvatar   string
	LastMsgID        uint64
	LastMsgContent   string
	LastMsgTime      *time.Time
	ClearBeforeMsgID uint64
	UnreadCount      int
	IsTop            bool
	IsMute           bool
}

// TableName 固定表名。
func (sessionRow) TableName() string { return "chat_session_models" }

// msgRow 对应 chat_msg_models。
type msgRow struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt
	SenderID   uint64
	ReceiverID uint64
	SessionID  string
	Content    string
	SendTime   time.Time
	ReadAt     *time.Time
	MsgStatus  int8
	MsgType    int8
}

// TableName 固定表名。
func (msgRow) TableName() string { return "chat_msg_models" }

// stateRow 对应 chat_msg_user_states。
type stateRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	MsgID     uint64
	UserID    uint64
	SessionID string
}

// TableName 固定表名。
func (stateRow) TableName() string { return "chat_msg_user_states" }

// Repo 实现 domain.Repository。
type Repo struct {
	db *gorm.DB
}

var _ domain.Repository = (*Repo)(nil)

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// SendMessage 实现 domain.Repository。
func (r *Repo) SendMessage(ctx context.Context, msg *domain.Message, senderDisplay, receiverDisplay domain.Display, summary string) error {
	if msg.ID == 0 {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		msg.ID = ctype.ID(next.Uint64())
	}
	msg.SessionID = domain.BuildSessionID(msg.SenderID, msg.ReceiverID)

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sessions := []sessionRow{{
			SessionID:        msg.SessionID,
			UserID:           uint64(msg.SenderID),
			ReceiverID:       uint64(msg.ReceiverID),
			ReceiverNickname: receiverDisplay.Nickname,
			ReceiverAvatar:   receiverDisplay.Avatar,
		}}
		if msg.SenderID != msg.ReceiverID {
			sessions = append(sessions, sessionRow{
				SessionID:        msg.SessionID,
				UserID:           uint64(msg.ReceiverID),
				ReceiverID:       uint64(msg.SenderID),
				ReceiverNickname: senderDisplay.Nickname,
				ReceiverAvatar:   senderDisplay.Avatar,
			})
		}
		// 本包的行结构是 adapter 私有的，没有 BaseRow 的 BeforeCreate 钩子，
		// 主键必须显式赋值。漏了就会把所有行都写成 id=0：
		// 第一条成功，第二条撞主键（ON DUPLICATE KEY 于是变成 update），
		// 表现是"整个站只有一条会话"。
		if err := fillRowIDs(sessions); err != nil {
			return err
		}
		// 先把"被删除的会话"的未读清零，再插入/恢复。
		//
		// 不能把清零写成 ON DUPLICATE KEY UPDATE 里的一句
		// `unread_count = CASE WHEN deleted_at IS NOT NULL THEN 0 ELSE unread_count END`：
		// 同一句里前面已经把 deleted_at 赋成 NULL，而 map 的赋值顺序不确定，
		// 所以 CASE 有时候读到的是"已被清空的 deleted_at"，于是保留了旧未读。
		// 实测症状是"删过的会话重新收到消息，未读数变成 3 而不是 1"，且时好时坏。
		// 拆成独立的 UPDATE 后顺序确定，与 map 赋值顺序无关。
		// 必须 Unscoped：sessionRow 带 gorm.DeletedAt，GORM 会自行为查询补
		// `deleted_at IS NULL`，正好把要处理的行排除掉 —— 语句不报错、影响 0 行。
		if err := tx.Unscoped().Model(&sessionRow{}).
			Where("session_id = ? AND deleted_at IS NOT NULL", msg.SessionID).
			Update("unread_count", 0).Error; err != nil {
			return err
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "receiver_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"session_id": msg.SessionID,
				// MySQL 没有 PostgreSQL 的 `excluded` 伪表：写 excluded.x 会被
				// 当成"不存在的列"报 Error 1054 (Unknown column)，发消息直接 500。
				// MySQL 取"待插入值"的写法是 VALUES(col)。
				"receiver_nickname": gorm.Expr("VALUES(receiver_nickname)"),
				"receiver_avatar":   gorm.Expr("VALUES(receiver_avatar)"),
				// 恢复会话：软删的会话重新收到消息时要回到列表里。
				"deleted_at": nil,
			}),
		}).Create(&sessions).Error; err != nil {
			return err
		}

		row := msgRow{
			ID:         msg.ID.Uint64(),
			SenderID:   uint64(msg.SenderID),
			ReceiverID: uint64(msg.ReceiverID),
			SessionID:  msg.SessionID,
			Content:    msg.Content,
			SendTime:   msg.SendTime,
			MsgStatus:  int8(msg.MsgStatus),
			MsgType:    int8(msg.MsgType),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		msg.ID = ctype.ID(row.ID)

		updates := map[string]any{
			"last_msg_id":      msg.ID,
			"last_msg_content": summary,
			"last_msg_time":    msg.SendTime,
		}
		expected := int64(2)
		if msg.SenderID == msg.ReceiverID {
			expected = 1
		} else {
			updates["unread_count"] = gorm.Expr(
				"CASE WHEN user_id = ? AND receiver_id = ? THEN unread_count + 1 ELSE unread_count END",
				msg.ReceiverID, msg.SenderID,
			)
		}
		res := tx.Model(&sessionRow{}).
			Where(`session_id = ? AND ((user_id = ? AND receiver_id = ?) OR (user_id = ? AND receiver_id = ?))`,
				msg.SessionID, msg.SenderID, msg.ReceiverID, msg.ReceiverID, msg.SenderID).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != expected {
			return fmt.Errorf("chat session update mismatch: session=%s affected=%d", msg.SessionID, res.RowsAffected)
		}
		return nil
	})
}

// fillRowIDs 给会话行补雪花主键。
//
// 为什么需要它：sessionRow 是本包私有的投影结构，不像 testkit/schema.ChatSessionRow
// 那样带 Model.BeforeCreate 钩子。GORM 不会替 `autoIncrement:false` 的主键取值，
// 于是所有行的 id 都是 0 —— 只写一行时看不出来，第二行就撞主键，
// 而 ON DUPLICATE KEY UPDATE 会把它变成 update，最终全站只剩一条会话。
func fillRowIDs(rows []sessionRow) error {
	for i := range rows {
		if rows[i].ID != 0 {
			continue
		}
		id, err := idgen.Next()
		if err != nil {
			return err
		}
		rows[i].ID = id.Uint64()
	}
	return nil
}

// ListSessions 实现 domain.Repository。
func (r *Repo) ListSessions(ctx context.Context, userID ctype.ID, unscoped bool, page domain.Page) ([]domain.Session, int, error) {
	page = page.Normalize()
	base := func() *gorm.DB {
		tx := r.db.WithContext(ctx).Model(&sessionRow{}).Where("user_id = ?", uint64(userID))
		if unscoped {
			tx = tx.Unscoped()
		}
		return tx
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []sessionRow
	if err := base().
		Select("session_id", "receiver_id", "receiver_nickname", "receiver_avatar",
			"last_msg_content", "last_msg_time", "unread_count", "is_top", "is_mute", "deleted_at").
		Order("is_top desc, last_msg_time desc, id desc").
		Limit(page.Limit).
		Offset((page.Page - 1) * page.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSession(row))
	}
	return out, int(total), nil
}

// FindSession 实现 domain.Repository。
func (r *Repo) FindSession(ctx context.Context, sessionID string, userID ctype.ID, unscoped bool) (domain.Session, error) {
	tx := r.db.WithContext(ctx)
	if unscoped {
		tx = tx.Unscoped()
	}
	var row sessionRow
	if err := tx.Select("session_id", "clear_before_msg_id").
		Take(&row, "session_id = ? AND user_id = ?", sessionID, uint64(userID)).Error; err != nil {
		return domain.Session{}, apperr.New(apperr.CodeChatSessionNotFound)
	}
	return toSession(row), nil
}

// ListMessages 实现 domain.Repository。
func (r *Repo) ListMessages(ctx context.Context, q domain.MessageQuery) ([]domain.Message, int, error) {
	page := q.Page.Normalize()
	base := func() *gorm.DB {
		tx := r.db.WithContext(ctx).Model(&msgRow{}).Where("session_id = ?", q.SessionID)
		if q.Unscoped {
			tx = tx.Unscoped()
		}
		if q.ClearBefore > 0 {
			tx = tx.Where("id > ?", uint64(q.ClearBefore))
		}
		if !q.Unscoped {
			sub := r.db.WithContext(ctx).Unscoped().Model(&stateRow{}).
				Select("msg_id").
				Where("user_id = ? AND session_id = ? AND deleted_at IS NOT NULL", uint64(q.UserID), q.SessionID)
			tx = tx.Where("id NOT IN (?)", sub)
		}
		return tx
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []msgRow
	if err := base().Order("send_time desc").
		Limit(page.Limit).Offset((page.Page - 1) * page.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMessage(row))
	}
	return out, int(total), nil
}

// UserDeletedAt 实现 domain.Repository。
func (r *Repo) UserDeletedAt(ctx context.Context, userID ctype.ID, sessionID string, ids []ctype.ID) (map[ctype.ID]time.Time, error) {
	out := make(map[ctype.ID]time.Time, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw := toRawIDs(ids)
	var rows []stateRow
	if err := r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND session_id = ? AND msg_id IN ? AND deleted_at IS NOT NULL", uint64(userID), sessionID, raw).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.DeletedAt.Valid {
			out[ctype.ID(row.MsgID)] = row.DeletedAt.Time
		}
	}
	return out, nil
}

// MarkRead 实现 domain.Repository。
func (r *Repo) MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var affected []domain.Message
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []msgRow
		if err := tx.Select("id", "session_id", "sender_id", "receiver_id").
			Where("id IN ? AND receiver_id = ? AND msg_status < ?", toRawIDs(ids), uint64(userID), int8(domain.StatusRead)).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		msgIDs := make([]uint64, 0, len(rows))
		sessionDelta := make(map[string]int, len(rows))
		for _, row := range rows {
			msgIDs = append(msgIDs, row.ID)
			sessionDelta[row.SessionID]++
			affected = append(affected, toMessage(row))
		}
		if err := tx.Model(&msgRow{}).Where("id IN ?", msgIDs).
			Updates(map[string]any{"msg_status": int8(domain.StatusRead), "read_at": &now}).Error; err != nil {
			return err
		}
		return decreaseUnread(tx, userID, sessionDelta)
	})
	if err != nil {
		return nil, err
	}
	return affected, nil
}

// DeleteMessagesForUser 实现 domain.Repository。
func (r *Repo) DeleteMessagesForUser(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []msgRow
	if err := r.db.WithContext(ctx).Select("id", "session_id").
		Where("id IN ? AND (sender_id = ? OR receiver_id = ?)", toRawIDs(ids), uint64(userID), uint64(userID)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	now := time.Now()
	states := make([]stateRow, 0, len(rows))
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		states = append(states, stateRow{
			DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
			MsgID:     row.ID,
			UserID:    uint64(userID),
			SessionID: row.SessionID,
		})
		out = append(out, toMessage(row))
	}
	// 同会话行：没有钩子兜底，主键必须自己填。漏填会让所有用户态行撞 id=0。
	for i := range states {
		id, genErr := idgen.Next()
		if genErr != nil {
			return nil, genErr
		}
		states[i].ID = id.Uint64()
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "msg_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"session_id", "deleted_at", "updated_at"}),
	}).Create(&states).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSessions 实现 domain.Repository。
func (r *Repo) DeleteSessions(ctx context.Context, userID ctype.ID, sessionIDs []string) ([]string, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	var deleted []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []sessionRow
		if err := tx.Select("id", "session_id", "clear_before_msg_id").
			Where("user_id = ? AND session_id IN ?", uint64(userID), sessionIDs).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		maxIDs, err := loadMaxMsgIDs(tx, sessionIDs)
		if err != nil {
			return err
		}
		idList := make([]uint64, 0, len(rows))
		caseSQL := "CASE id"
		args := make([]any, 0, len(rows)*2)
		for _, row := range rows {
			clear := row.ClearBeforeMsgID
			if maxIDs[row.SessionID] > clear {
				clear = maxIDs[row.SessionID]
			}
			idList = append(idList, row.ID)
			caseSQL += " WHEN ? THEN ?"
			args = append(args, row.ID, clear)
			deleted = append(deleted, row.SessionID)
		}
		caseSQL += " ELSE clear_before_msg_id END"
		if err := tx.Model(&sessionRow{}).Where("id IN ?", idList).
			Updates(map[string]any{
				"clear_before_msg_id": gorm.Expr(caseSQL, args...),
				"unread_count":        0,
			}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", idList).Delete(&sessionRow{}).Error
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

func decreaseUnread(tx *gorm.DB, userID ctype.ID, delta map[string]int) error {
	if len(delta) == 0 {
		return nil
	}
	caseSQL := "CASE session_id"
	args := make([]any, 0, len(delta)*3)
	sessionIDs := make([]string, 0, len(delta))
	for sessionID, n := range delta {
		caseSQL += " WHEN ? THEN CASE WHEN unread_count >= ? THEN unread_count - ? ELSE 0 END"
		args = append(args, sessionID, n, n)
		sessionIDs = append(sessionIDs, sessionID)
	}
	caseSQL += " ELSE unread_count END"
	return tx.Unscoped().Model(&sessionRow{}).
		Where("user_id = ? AND session_id IN ?", uint64(userID), sessionIDs).
		Update("unread_count", gorm.Expr(caseSQL, args...)).Error
}

func loadMaxMsgIDs(tx *gorm.DB, sessionIDs []string) (map[string]uint64, error) {
	var rows []struct {
		SessionID string
		MaxMsgID  uint64
	}
	if err := tx.Model(&msgRow{}).
		Select("session_id, MAX(id) AS max_msg_id").
		Where("session_id IN ?", sessionIDs).
		Group("session_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]uint64, len(rows))
	for _, row := range rows {
		out[row.SessionID] = row.MaxMsgID
	}
	return out, nil
}

func toRawIDs(ids []ctype.ID) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			out = append(out, uint64(id))
		}
	}
	return out
}

func toSession(row sessionRow) domain.Session {
	return domain.Session{
		ID:               row.ID,
		SessionID:        row.SessionID,
		UserID:           ctype.ID(row.UserID),
		ReceiverID:       ctype.ID(row.ReceiverID),
		ReceiverNickname: row.ReceiverNickname,
		ReceiverAvatar:   row.ReceiverAvatar,
		LastMsgContent:   row.LastMsgContent,
		LastMsgTime:      row.LastMsgTime,
		ClearBeforeMsgID: ctype.ID(row.ClearBeforeMsgID),
		UnreadCount:      row.UnreadCount,
		IsTop:            row.IsTop,
		IsMute:           row.IsMute,
		DeletedAt:        rowDeletedAt(row),
	}
}

func toMessage(row msgRow) domain.Message {
	return domain.Message{
		ID:         ctype.ID(row.ID),
		SenderID:   ctype.ID(row.SenderID),
		ReceiverID: ctype.ID(row.ReceiverID),
		SessionID:  row.SessionID,
		Content:    row.Content,
		SendTime:   row.SendTime,
		ReadAt:     row.ReadAt,
		MsgStatus:  domain.MsgStatus(row.MsgStatus),
		MsgType:    domain.MsgType(row.MsgType),
	}
}

func rowDeletedAt(row sessionRow) *time.Time {
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		return &t
	}
	return nil
}
