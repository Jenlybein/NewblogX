// Package mysql 是私信模块的 MySQL 适配器。
package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/chat/domain/msgid"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"
)

// sessionRow 对应 chat_session_models。
type sessionRow struct {
	ID               ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	SessionID        string
	UserID           ctype.ID
	ReceiverID       ctype.ID
	ReceiverNickname string
	ReceiverAvatar   string
	LastMsgID        ctype.ID
	LastMsgContent   string
	LastMsgTime      *time.Time
	ClearBeforeMsgID ctype.ID
	UnreadCount      int
	IsTop            bool
	IsMute           bool
}

// TableName 固定表名。
func (sessionRow) TableName() string { return "chat_session_models" }

// msgRow 对应 chat_msg_models。
type msgRow struct {
	ID         ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt
	SenderID   ctype.ID
	ReceiverID ctype.ID
	SessionID  string
	Content    string
	SendTime   time.Time
	ReadAt     *time.Time
	MsgStatus  int8
	MsgType    int8
}

// TableName 固定表名。
func (msgRow) TableName() string { return "chat_msg_models" }

// stateRow 对应 chat_msg_user_state_models。
type stateRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	MsgID     ctype.ID
	UserID    ctype.ID
	SessionID string
}

// TableName 固定表名。
//
// 必须与 schema.sql 的 chat_msg_user_state_models 逐字一致：这张表既被读
// （ListMessages 过滤子查询、UserDeletedAt）又被写（DeleteMessagesForUser），
// 名字漂移在生产就是 Error 1146。守卫见 testkit/schema 的表名静态测试。
func (stateRow) TableName() string { return "chat_msg_user_state_models" }

// Repo 实现 domain.Repository。
type Repo struct {
	db *gorm.DB
}

var (
	_ domain.Query   = (*Repo)(nil)
	_ domain.Command = (*Repo)(nil)
)

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// errDuplicateClientMsg 是"命中幂等键、该消息已存在"的内部信号。
//
// 只在事务内使用：返回它让事务回滚（不产生第二条消息、也不重复加未读），
// 再由 Send 在同 ID 上读回第一次落库的那条。
var errDuplicateClientMsg = errors.New("chat: duplicate client msg key")

// Send 实现 domain.Command。
//
// 幂等：cmd.ClientMsgID 非空时，主键由（发送人，幂等键）派生（见
// domain.MessageIDFromClientKey）。重复提交会撞主键，整个事务回滚，
// 然后读回第一次落库的消息并标记 duplicated=true。
// 这就是"零 DDL 下的幂等落库"：幂等性由数据库唯一约束保证，
// 不依赖进程内缓存、不依赖 Redis、多实例并发也成立。
func (r *Repo) Send(ctx context.Context, cmd domain.SendCommand, senderDisplay, receiverDisplay domain.Display, summary string) (domain.Message, bool, error) {
	msg := domain.Message{
		SenderID:   cmd.SenderID,
		ReceiverID: cmd.ReceiverID,
		SessionID:  domain.BuildSessionID(cmd.SenderID, cmd.ReceiverID),
		Content:    cmd.Content,
		SendTime:   cmd.SendTime,
		MsgStatus:  domain.StatusSent,
		MsgType:    cmd.MsgType,
	}
	if cmd.ClientMsgID != "" {
		msg.ID = msgid.FromClientKey(cmd.SenderID, cmd.ClientMsgID)
	} else {
		next, err := idgen.Next()
		if err != nil {
			return domain.Message{}, false, err
		}
		msg.ID = ctype.NewID(next.Uint64())
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sessions := []sessionRow{{
			SessionID:        msg.SessionID,
			UserID:           msg.SenderID,
			ReceiverID:       msg.ReceiverID,
			ReceiverNickname: receiverDisplay.Nickname,
			ReceiverAvatar:   receiverDisplay.Avatar,
		}}
		if msg.SenderID != msg.ReceiverID {
			sessions = append(sessions, sessionRow{
				SessionID:        msg.SessionID,
				UserID:           msg.ReceiverID,
				ReceiverID:       msg.SenderID,
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
			ID:         msg.ID,
			SenderID:   msg.SenderID,
			ReceiverID: msg.ReceiverID,
			SessionID:  msg.SessionID,
			Content:    msg.Content,
			SendTime:   msg.SendTime,
			MsgStatus:  int8(msg.MsgStatus),
			MsgType:    int8(msg.MsgType),
		}
		if err := tx.Create(&row).Error; err != nil {
			// 幂等键命中：主键已存在。把整个事务回滚，交给调用方读回既有消息。
			if cmd.ClientMsgID != "" && isDuplicateKey(err) {
				return errDuplicateClientMsg
			}
			return err
		}
		msg.ID = ctype.NewID(row.ID.Uint64())

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

		// 写副作用走发件箱：同事务落一条"消息已发送"事件，
		// 由中继的订阅者异步推 WS 通知（HTTP 落库是唯一真相，WS 只是通知）。
		return emitEvent(tx, domain.EventMessageSent, msg.ID, domain.SentNotice{
			MessageID:  msg.ID,
			SenderID:   msg.SenderID,
			ReceiverID: msg.ReceiverID,
			SessionID:  msg.SessionID,
			MsgType:    msg.MsgType,
			SendTime:   msg.SendTime,
		})
	})
	if err != nil {
		// 幂等命中：读回第一次落库的消息，调用方据此回滚额度预占。
		if errors.Is(err, errDuplicateClientMsg) {
			existing, loadErr := r.findMessage(ctx, msg.ID)
			if loadErr != nil {
				return domain.Message{}, false, loadErr
			}
			if existing.SenderID != cmd.SenderID {
				return domain.Message{}, false, apperr.New(apperr.CodeChatSendFailed)
			}
			return existing, true, nil
		}
		return domain.Message{}, false, err
	}
	return msg, false, nil
}

// findMessage 按主键读一条消息（幂等重放时读回既有那条）。
func (r *Repo) findMessage(ctx context.Context, id ctype.ID) (domain.Message, error) {
	var row msgRow
	if err := r.db.WithContext(ctx).Unscoped().Take(&row, "id = ?", id.Uint64()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Message{}, apperr.New(apperr.CodeChatSendFailed)
		}
		return domain.Message{}, err
	}
	return toMessage(row), nil
}

// isDuplicateKey 识别唯一键冲突。
//
// 生产（core/init_db.go）与测试库（testkit/mysqlcontainer）都开了
// gorm.Config.TranslateError，所以冲突统一表现为 gorm.ErrDuplicatedKey。
// 全仓的判重都只认这一个错误（见 article 的 taxonomy_repo / collection_repo），
// 这里保持一致；若哪天有调用方漏开翻译，它会退化成普通失败而不是静默丢幂等。
func isDuplicateKey(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// fillRowIDs 给会话行补雪花主键。
//
// 为什么需要它：sessionRow 是本包私有的投影结构，不像 testkit/schema.ChatSessionRow
// 那样带 Model.BeforeCreate 钩子。GORM 不会替 `autoIncrement:false` 的主键取值，
// 于是所有行的 id 都是 0 —— 只写一行时看不出来，第二行就撞主键，
// 而 ON DUPLICATE KEY UPDATE 会把它变成 update，最终全站只剩一条会话。
func fillRowIDs(rows []sessionRow) error {
	for i := range rows {
		if !rows[i].ID.IsZero() {
			continue
		}
		id, err := idgen.Next()
		if err != nil {
			return err
		}
		rows[i].ID = id
	}
	return nil
}

// ListSessions 实现 domain.Repository。
func (r *Repo) ListSessions(ctx context.Context, userID ctype.ID, unscoped bool, page domain.Page) ([]domain.Session, int, error) {
	page = page.Normalize()
	base := func() *gorm.DB {
		tx := r.db.WithContext(ctx).Model(&sessionRow{}).Where("user_id = ?", userID.Uint64())
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
		Take(&row, "session_id = ? AND user_id = ?", sessionID, userID.Uint64()).Error; err != nil {
		return domain.Session{}, apperr.New(apperr.CodeChatSessionNotFound)
	}
	return toSession(row), nil
}

// ListMessages 实现 domain.Query。
func (r *Repo) ListMessages(ctx context.Context, q domain.MessageQuery) (domain.MessageQueryResult, error) {
	page := q.Page.Normalize()
	// 游标必须成对：只有时间没 id（或反过来）时，解析/校验不能静默退回第一页，
	// 否则前端"加载更早"会在原地反复拿到同一页。
	cursorActive := q.Cursor.BeforeSendTime != "" || !q.Cursor.BeforeID.IsZero()
	var cursorTime time.Time
	if cursorActive {
		if q.Cursor.BeforeSendTime == "" || q.Cursor.BeforeID.IsZero() {
			return domain.MessageQueryResult{}, apperr.New(apperr.CodeCommonInvalidArgument)
		}
		parsed, err := time.Parse(time.RFC3339Nano, q.Cursor.BeforeSendTime)
		if err != nil {
			return domain.MessageQueryResult{}, apperr.Wrap(apperr.CodeCommonInvalidArgument, err)
		}
		cursorTime = parsed
	}
	scope := visibilityScope{ctx: ctx, db: r.db, sessionID: q.SessionID, userID: q.UserID}.withClearBefore(q.ClearBefore)
	base := func() *gorm.DB { return scope.visible(q.Unscoped) }
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return domain.MessageQueryResult{}, err
	}
	// 多取一条判断 has_more：不能用 COUNT - offset 推，因为游标过滤后
	// COUNT 是"会话可见总数"，不是"本页之后还剩几条"。
	query := base().
		// id 只是相同 send_time 的确定性定序键，绝不是时序键：客户端幂等键
		// 派生的主键是 [2^62,2^63) 区间的 FNV 哈希，与消息先后无关。
		Order("send_time DESC, id DESC")
	if cursorActive {
		query = query.Where("(send_time < ? OR (send_time = ? AND id < ?))",
			cursorTime, cursorTime, q.Cursor.BeforeID.Uint64())
	} else {
		// 不带游标时保留既有 (page-1)*limit 偏移：管理员 / type=2 仍按页翻。
		query = query.Offset((page.Page - 1) * page.Limit)
	}
	var rows []msgRow
	if err := query.Limit(page.Limit + 1).Find(&rows).Error; err != nil {
		return domain.MessageQueryResult{}, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMessage(row))
	}
	result := domain.MessageQueryResult{
		Messages: out,
		Total:    int(total),
		HasMore:  hasMore,
	}
	// has_more=false 时 next_cursor 保持零值（空字符串），前端据此停翻。
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		result.NextCursor = domain.MessageCursor{
			BeforeSendTime: last.SendTime.Format(time.RFC3339Nano),
			BeforeID:       ctype.NewID(last.ID.Uint64()),
		}
	}
	return result, nil
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
		Where("user_id = ? AND session_id = ? AND msg_id IN ? AND deleted_at IS NOT NULL", userID.Uint64(), sessionID, raw).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.DeletedAt.Valid {
			out[ctype.NewID(row.MsgID.Uint64())] = row.DeletedAt.Time
		}
	}
	return out, nil
}

// MarkRead 实现 domain.Command。
func (r *Repo) MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return r.markReadWhere(ctx, userID, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("id IN ?", toRawIDs(ids))
	})
}

// MarkReadSession 实现 domain.Command：整条会话一次性已读。
//
// 与按 ID 列表的路径共用同一个事务体，差别只在筛选条件：这里按 session_id
// 圈定、并套用与消息列表一致的"时间水位 + 用户级删除"可见性，免得被清空的
// 历史消息因为一次"整会话已读"被重新计数 / 发回执。
func (r *Repo) MarkReadSession(ctx context.Context, userID ctype.ID, sessionID string) ([]domain.Message, error) {
	if sessionID == "" {
		return nil, apperr.New(apperr.CodeCommonInvalidArgument)
	}
	scope, err := r.newVisibilityScope(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	return r.markReadWhere(ctx, userID, func(tx *gorm.DB) *gorm.DB {
		// 用同一份可见集口径追加条件：会话、清空时间水位、用户级软删。
		return scope.clauses(tx.Where("session_id = ?", sessionID), false)
	})
}

// markReadWhere 是两种"标记已读"路径共用的事务体。
//
// scope 只负责追加筛选条件；主体统一做：选未读 → 更新状态与 read_at →
// 按会话递减未读 → 按（发送人, 会话）聚合成 chat.message.read 发件箱事件。
// 没有命中行时直接返回空结果，不是错误（第二次调用必须幂等）。
func (r *Repo) markReadWhere(ctx context.Context, userID ctype.ID, scope func(*gorm.DB) *gorm.DB) ([]domain.Message, error) {
	var affected []domain.Message
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&msgRow{}).
			Select("id", "session_id", "sender_id", "receiver_id").
			Where("receiver_id = ? AND msg_status < ?", userID, int8(domain.StatusRead))
		var rows []msgRow
		if err := scope(query).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		msgIDs := make([]uint64, 0, len(rows))
		sessionDelta := make(map[string]int, len(rows))
		// 已读回执是"写副作用"，按（发送人, 会话）聚合后走发件箱，
		// 不在事务内直接推连接池（推送失败/进程崩溃都不会丢回执）。
		type readKey struct {
			senderID  ctype.ID
			sessionID string
		}
		readGroups := make(map[readKey][]ctype.ID, len(rows))
		readOrder := make([]readKey, 0, len(rows))
		for _, row := range rows {
			msgIDs = append(msgIDs, row.ID.Uint64())
			sessionDelta[row.SessionID]++
			affected = append(affected, toMessage(row))

			key := readKey{senderID: ctype.NewID(row.SenderID.Uint64()), sessionID: row.SessionID}
			if _, ok := readGroups[key]; !ok {
				readOrder = append(readOrder, key)
			}
			readGroups[key] = append(readGroups[key], ctype.NewID(row.ID.Uint64()))
		}
		if err := tx.Model(&msgRow{}).Where("id IN ?", msgIDs).
			Updates(map[string]any{"msg_status": int8(domain.StatusRead), "read_at": &now}).Error; err != nil {
			return err
		}
		if err := decreaseUnread(tx, userID, sessionDelta); err != nil {
			return err
		}
		for _, key := range readOrder {
			idList := readGroups[key]
			if err := emitEvent(tx, domain.EventMessagesRead, idList[0], domain.ReadEvent{
				ReaderID:  userID,
				SenderID:  key.senderID,
				SessionID: key.sessionID,
				MsgIDList: idList,
				ReadAt:    now,
			}); err != nil {
				return err
			}
		}
		return nil
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
		Where("id IN ? AND (sender_id = ? OR receiver_id = ?)", toRawIDs(ids), userID.Uint64(), userID.Uint64()).
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
			UserID:    userID,
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
		states[i].ID = id
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
			Where("user_id = ? AND session_id IN ?", userID.Uint64(), sessionIDs).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		latest, err := loadLatestMessages(tx, sessionIDs)
		if err != nil {
			return err
		}
		watermarkTimes, err := loadWatermarkSendTimes(tx, rows)
		if err != nil {
			return err
		}
		idList := make([]uint64, 0, len(rows))
		caseSQL := "CASE id"
		args := make([]any, 0, len(rows)*2)
		for _, row := range rows {
			clear := row.ClearBeforeMsgID
			candidate, ok := latest[row.SessionID]
			if ok && candidate.after(watermarkTimes, clear) {
				clear = candidate.ID
			}
			idList = append(idList, row.ID.Uint64())
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
		Where("user_id = ? AND session_id IN ?", userID.Uint64(), sessionIDs).
		Update("unread_count", gorm.Expr(caseSQL, args...)).Error
}

func toRawIDs(ids []ctype.ID) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			out = append(out, id.Uint64())
		}
	}
	return out
}

func toSession(row sessionRow) domain.Session {
	return domain.Session{
		ID:               row.ID,
		SessionID:        row.SessionID,
		UserID:           ctype.NewID(row.UserID.Uint64()),
		ReceiverID:       ctype.NewID(row.ReceiverID.Uint64()),
		ReceiverNickname: row.ReceiverNickname,
		ReceiverAvatar:   row.ReceiverAvatar,
		LastMsgContent:   row.LastMsgContent,
		LastMsgTime:      row.LastMsgTime,
		ClearBeforeMsgID: ctype.NewID(row.ClearBeforeMsgID.Uint64()),
		UnreadCount:      row.UnreadCount,
		IsTop:            row.IsTop,
		IsMute:           row.IsMute,
		DeletedAt:        rowDeletedAt(row),
	}
}

func toMessage(row msgRow) domain.Message {
	return domain.Message{
		ID:         ctype.NewID(row.ID.Uint64()),
		SenderID:   ctype.NewID(row.SenderID.Uint64()),
		ReceiverID: ctype.NewID(row.ReceiverID.Uint64()),
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
