//go:build integration

package mysql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"myblogx/internal/chat/adapter/mysql"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// setup 建一套私信测试库。
//
// 必须带上 UserCore：chat_session_models.user_id 有指向 user_models 的外键。
// 虽然用户展示信息在会话里是**快照列**（receiver_nickname 等）、由调用方传入，
// 但会话行本身要求 user_id 在 user_models 里存在。
func setup(t *testing.T) (*mysql.Repo, *gorm.DB) {
	t.Helper()
	db := mysqlcontainer.DB(t)
	// 这里建的是**带真实外键**的表，关外键会让这些用例失去意义。
	//
	// 背景：ChatMsgModel 里原来有条 hasMany（SessionList），让 GORM 在
	// chat_session_models.session_id 上建出指向 chat_msg_models 的外键。
	// 发消息的顺序是"先建会话行、再插消息行"，那条外键让第一步必然违反
	// （Error 1452），以前只能靠关外键绕过。该字段已删除，
	// 所以现在的外键集是真实可满足的 —— "发消息能成功"本身就是证明。
	schema.Migrate(t, db, schema.UserCore, schema.Chat)
	for _, id := range []uint64{100, 200, 300} {
		schema.InsertUser(t, db, schema.UserSpec{
			ID:       id,
			Username: "u" + strconv.FormatUint(id, 10),
			Nickname: "用户" + strconv.FormatUint(id, 10),
		})
	}
	return mysql.New(db), db
}

// sendText 发一条文本消息，返回落库后的消息。
func sendText(t *testing.T, repo *mysql.Repo, from, to uint64, content string, at time.Time) *domain.Message {
	t.Helper()
	msg := &domain.Message{
		SenderID:   ctype.ID(from),
		ReceiverID: ctype.ID(to),
		Content:    content,
		SendTime:   at,
		MsgStatus:  domain.StatusSent,
		MsgType:    domain.TypeText,
	}
	if err := repo.SendMessage(context.Background(), msg,
		domain.Display{Nickname: "甲", Avatar: "a.png"},
		domain.Display{Nickname: "乙", Avatar: "b.png"},
		domain.Summary(domain.TypeText, content),
	); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}
	return msg
}

// sessionOf 读某用户视角下的一条会话。
func sessionOf(t *testing.T, db *gorm.DB, userID uint64, sessionID string) (unread int, lastContent string, lastMsgID uint64, deleted bool) {
	t.Helper()
	var row struct {
		UnreadCount    int
		LastMsgContent string
		LastMsgID      uint64
		DeletedAt      *time.Time
	}
	err := db.Table("chat_session_models").
		Select("unread_count, last_msg_content, last_msg_id, deleted_at").
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Take(&row).Error
	if err != nil {
		t.Fatalf("读会话失败 user=%d session=%s: %v", userID, sessionID, err)
	}
	return row.UnreadCount, row.LastMsgContent, row.LastMsgID, row.DeletedAt != nil
}

// TestSendMessageCreatesBothSidesAndCountsUnread 发一条消息应同时：
// 给双方各建一条会话、接收方未读 +1、发送方未读不变、双方 last_msg 都更新。
//
// 这是私信最核心的性质：只写一半会表现为"对方收不到 / 红点不出现"，
// 而接口照样返回成功 —— 只能靠真库断出来。
func TestSendMessageCreatesBothSidesAndCountsUnread(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	msg := sendText(t, repo, 100, 200, "在吗", at)

	sessionID := domain.BuildSessionID(100, 200)
	// session_id 与方向无关：双方看到的是同一个 id。
	if sessionID != domain.BuildSessionID(200, 100) {
		t.Fatalf("session_id 应双向一致，got=%s", sessionID)
	}

	var count int64
	if err := db.Table("chat_session_models").Count(&count).Error; err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("双方应各有一条会话，实际 %d 条", count)
	}

	unreadReceiver, contentReceiver, lastID, _ := sessionOf(t, db, 200, sessionID)
	if unreadReceiver != 1 {
		t.Fatalf("接收方未读应为 1，实际 %d", unreadReceiver)
	}
	if contentReceiver != "在吗" {
		t.Fatalf("接收方 last_msg_content 应为原文，实际 %q", contentReceiver)
	}
	if lastID != msg.ID.Uint64() {
		t.Fatalf("接收方 last_msg_id 应为新消息 id=%d，实际 %d", msg.ID, lastID)
	}

	unreadSender, _, _, _ := sessionOf(t, db, 100, sessionID)
	if unreadSender != 0 {
		t.Fatalf("发送方未读不应增加，实际 %d", unreadSender)
	}
}

// TestSendMessageTwiceReusesSession 同一对用户再发一条消息，不应新增会话行。
//
// 靠 (user_id, receiver_id) 唯一键 + OnConflict 实现。
// 若唯一键写错，表现是"会话列表里同一个人的对话越来越多条"。
func TestSendMessageTwiceReusesSession(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sendText(t, repo, 100, 200, "第一条", base)
	sendText(t, repo, 100, 200, "第二条", base.Add(time.Minute))

	var count int64
	if err := db.Table("chat_session_models").Count(&count).Error; err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("重复发送不该新增会话行，实际 %d 条（期望 2）", count)
	}

	unread, content, _, _ := sessionOf(t, db, 200, domain.BuildSessionID(100, 200))
	if unread != 2 {
		t.Fatalf("接收方未读应累计到 2，实际 %d", unread)
	}
	if content != "第二条" {
		t.Fatalf("last_msg_content 应为最后一条，实际 %q", content)
	}
}

// TestSendMessageRevivesDeletedSession 对方删掉会话后再发消息，会话应被恢复且未读从 0 重算。
//
// 语义来自 OnConflict 里的
// `unread_count = CASE WHEN deleted_at IS NOT NULL THEN 0 ELSE unread_count END`：
// 被删会话的旧未读不该继续累积（用户已经"清空"过一次）。
// 若这条错成 `unread_count + 1`，用户会看到删干净的对话里突然冒出 10 条未读。
func TestSendMessageRevivesDeletedSession(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sendText(t, repo, 100, 200, "第一条", base)
	sendText(t, repo, 100, 200, "第二条", base.Add(time.Minute))

	sessionID := domain.BuildSessionID(100, 200)
	// 接收方软删会话。
	if err := db.Exec("UPDATE chat_session_models SET deleted_at = ? WHERE user_id = ? AND session_id = ?",
		base, 200, sessionID).Error; err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}

	sendText(t, repo, 100, 200, "第三条", base.Add(2*time.Minute))

	unread, content, _, deleted := sessionOf(t, db, 200, sessionID)
	if deleted {
		t.Fatal("再次发送应恢复被删会话（deleted_at 置空）")
	}
	if unread != 1 {
		t.Fatalf("恢复后未读应从 0 重新计 1，实际 %d（旧未读被累加了）", unread)
	}
	if content != "第三条" {
		t.Fatalf("last_msg_content 应为新消息，实际 %q", content)
	}
}

// TestMarkReadDecreasesUnreadAndIsIdempotent 标记已读要递减未读，且重复标记不再变化。
//
// 幂等很关键：前端"进入会话"和"滚动到底"都会触发标记已读，
// 重复调用若再减一次，红点会变成负数或被提前清空。
func TestMarkReadDecreasesUnreadAndIsIdempotent(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m1 := sendText(t, repo, 100, 200, "一", base)
	m2 := sendText(t, repo, 100, 200, "二", base.Add(time.Minute))

	ctx := context.Background()
	sessionID := domain.BuildSessionID(100, 200)

	affected, err := repo.MarkRead(ctx, 200, []ctype.ID{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	if len(affected) != 2 {
		t.Fatalf("应命中 2 条未读消息，实际 %d", len(affected))
	}
	unread, _, _, _ := sessionOf(t, db, 200, sessionID)
	if unread != 0 {
		t.Fatalf("标记已读后未读应归零，实际 %d", unread)
	}

	// 再来一次：已读消息不该被重新计入。
	again, err := repo.MarkRead(ctx, 200, []ctype.ID{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("重复标记已读失败: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("重复标记应命中 0 条，实际 %d", len(again))
	}
	unread, _, _, _ = sessionOf(t, db, 200, sessionID)
	if unread != 0 {
		t.Fatalf("重复标记后未读不该变负，实际 %d", unread)
	}
}

// TestMarkReadIgnoresOthersMessages 只能标记"发给自己的"消息。
//
// 少这个条件的话，任何人都能把别人的未读清掉（越权 + 红点消失）。
func TestMarkReadIgnoresOthersMessages(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	msg := sendText(t, repo, 100, 200, "给乙的", base)

	// 让 300 去标记一条不是发给他的消息。
	affected, err := repo.MarkRead(context.Background(), 300, []ctype.ID{msg.ID})
	if err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	if len(affected) != 0 {
		t.Fatalf("不该能标记别人收到的消息，实际命中 %d 条", len(affected))
	}

	// 接收方未读仍在。
	unread, _, _, _ := sessionOf(t, db, 200, domain.BuildSessionID(100, 200))
	if unread != 1 {
		t.Fatalf("越权标记不该影响接收方未读，实际 %d", unread)
	}
}

// TestListSessionsOrdersByLastMsgTime 会话列表按最后消息时间倒序。
//
// 顺序错的表现很隐蔽：列表能用，但"最新聊的人在中间"，
// 用户会以为消息没发出去。
func TestListSessionsOrdersByLastMsgTime(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// 100 先和 200 聊，再和 300 聊；300 应该排前面。
	sendText(t, repo, 100, 200, "老对话", base)
	sendText(t, repo, 100, 300, "新对话", base.Add(time.Hour))

	rows, total, err := repo.ListSessions(context.Background(), 100, false, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查会话列表失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("应有 2 条会话，实际 %d", total)
	}
	if len(rows) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(rows))
	}
	if rows[0].ReceiverID != 300 {
		t.Fatalf("最新对话应排第一（receiver=300），实际 receiver=%d", rows[0].ReceiverID)
	}
	_ = db
}
