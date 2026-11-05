//go:build integration

package mysql_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"myblogx/internal/chat/adapter/mysql"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	notifmysql "myblogx/internal/notification/adapter/mysql"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// setup 建一套私信测试库。
//
// 必须带上 UserCore：chat_session_models.user_id 有指向 user_models 的外键。
// 虽然用户展示信息在会话里是**快照列**（receiver_nickname 等）、由调用方传入，
// 但会话行本身要求 user_id 在 user_models 里存在。
//
// 必须带上 Outbox：发送与已读会在**同一个事务**里写发件箱事件（#201）。
// 不建这张表的话发送会直接报"表不存在"，而这恰恰是要守住的
// "副作用与业务同事务"的性质。
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
	schema.Migrate(t, db, schema.UserCore, schema.Chat, schema.Outbox)
	for _, id := range []uint64{100, 200, 300} {
		schema.InsertUser(t, db, schema.UserSpec{
			ID:       id,
			Username: "u" + strconv.FormatUint(id, 10),
			Nickname: "用户" + strconv.FormatUint(id, 10),
		})
	}
	return mysql.New(db), db
}

// sendText 发一条文本消息，返回落库后的消息与"是否幂等命中"。
func sendText(t *testing.T, repo *mysql.Repo, from, to uint64, content string, at time.Time) (*domain.Message, bool) {
	t.Helper()
	return sendTextWithKey(t, repo, from, to, content, "", at)
}

// sendTextWithKey 发一条文本消息；clientKey 非空时启用幂等。
func sendTextWithKey(t *testing.T, repo *mysql.Repo, from, to uint64, content, clientKey string, at time.Time) (*domain.Message, bool) {
	t.Helper()
	msg, duplicated, err := repo.Send(context.Background(), domain.SendCommand{
		SenderID:    ctype.NewID(from),
		ReceiverID:  ctype.NewID(to),
		Content:     content,
		ClientMsgID: clientKey,
		SendTime:    at,
		MsgType:     domain.TypeText,
	},
		domain.Display{Nickname: "甲", Avatar: "a.png"},
		domain.Display{Nickname: "乙", Avatar: "b.png"},
		domain.Summary(domain.TypeText, content),
	)
	if err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}
	return &msg, duplicated
}

// sessionOf 读某用户视角下的一条会话。
func sessionOf(t *testing.T, db *gorm.DB, userID uint64, sessionID string) (unread int, lastContent string, lastMsgID uint64, deleted bool) {
	t.Helper()
	var row struct {
		UnreadCount    int
		LastMsgContent string
		LastMsgID      ctype.ID
		DeletedAt      *time.Time
	}
	err := db.Table("chat_session_models").
		Select("unread_count, last_msg_content, last_msg_id, deleted_at").
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Take(&row).Error
	if err != nil {
		t.Fatalf("读会话失败 user=%d session=%s: %v", userID, sessionID, err)
	}
	return row.UnreadCount, row.LastMsgContent, row.LastMsgID.Uint64(), row.DeletedAt != nil
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
	msg, _ := sendText(t, repo, 100, 200, "在吗", at)

	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))
	// session_id 与方向无关：双方看到的是同一个 id。
	if sessionID != domain.BuildSessionID(ctype.NewID(200), ctype.NewID(100)) {
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
		t.Fatalf("接收方 last_msg_id 应为新消息 id=%s，实际 %d", msg.ID, lastID)
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

	unread, content, _, _ := sessionOf(t, db, 200, domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200)))
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

	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))
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
	m1, _ := sendText(t, repo, 100, 200, "一", base)
	m2, _ := sendText(t, repo, 100, 200, "二", base.Add(time.Minute))

	ctx := context.Background()
	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))

	affected, err := repo.MarkRead(ctx, ctype.NewID(200), []ctype.ID{m1.ID, m2.ID})
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
	again, err := repo.MarkRead(ctx, ctype.NewID(200), []ctype.ID{m1.ID, m2.ID})
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
	msg, _ := sendText(t, repo, 100, 200, "给乙的", base)

	// 让 300 去标记一条不是发给他的消息。
	affected, err := repo.MarkRead(context.Background(), ctype.NewID(300), []ctype.ID{msg.ID})
	if err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	if len(affected) != 0 {
		t.Fatalf("不该能标记别人收到的消息，实际命中 %d 条", len(affected))
	}

	// 接收方未读仍在。
	unread, _, _, _ := sessionOf(t, db, 200, domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200)))
	if unread != 1 {
		t.Fatalf("越权标记不该影响接收方未读，实际 %d", unread)
	}
}

// TestDeleteMessagesForUserWritesUserState 删除消息要真的写进用户状态表：
// 删除只对发起者生效（对方仍能看到），删除后再查消息列表不再返回它们。
//
// # 为什么这条用例必须存在
//
// 状态表是唯一一张既被读（ListMessages 的过滤子查询、管理端 UserDeletedAt）
// 又被写的表，而原来的 6 个 chat 集成用例没有一个走到写入路径。表名写错时
// 它们全绿，生产第一次点"删除消息"就 Error 1146。这里从公开仓储接口走完
// 读-写-再读，表名漂移会立刻暴露。
func TestDeleteMessagesForUserWritesUserState(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m1, _ := sendText(t, repo, 100, 200, "一", base)
	m2, _ := sendText(t, repo, 100, 200, "二", base.Add(time.Minute))
	m3, _ := sendText(t, repo, 100, 200, "三", base.Add(2*time.Minute))
	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))
	ctx := context.Background()

	deleted, err := repo.DeleteMessagesForUser(ctx, ctype.NewID(200), []ctype.ID{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("删除消息失败（状态表名/写入路径可能不对）: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("应命中 2 条可删消息，实际 %d", len(deleted))
	}

	// 状态行确实落在真表的软删列上。
	if n := countUserStateRows(t, db, 200, sessionID); n != 2 {
		t.Fatalf("状态表应有 2 条删除记录，实际 %d", n)
	}

	// 发起者视角：被删的两条不再出现，第三条仍在。
	got, err := repo.ListMessages(ctx, domain.MessageQuery{
		UserID: ctype.NewID(200), SessionID: sessionID, Page: domain.Page{Page: 1, Limit: 10},
	})
	if err != nil {
		t.Fatalf("删除后查消息列表失败: %v", err)
	}
	if got.Total != 1 || len(got.Messages) != 1 || got.Messages[0].ID != m3.ID {
		t.Fatalf("删除后应只剩 1 条未删消息（m3=%s），实际 total=%d len=%d", m3.ID, got.Total, len(got.Messages))
	}

	// 对方视角不变：用户级删除不动原消息。
	other, err := repo.ListMessages(ctx, domain.MessageQuery{
		UserID: ctype.NewID(100), SessionID: sessionID, Page: domain.Page{Page: 1, Limit: 10},
	})
	if err != nil {
		t.Fatalf("对方查消息列表失败: %v", err)
	}
	if other.Total != 3 || len(other.Messages) != 3 {
		t.Fatalf("发送方仍应看到 3 条，实际 total=%d len=%d", other.Total, len(other.Messages))
	}

	// 管理端读删除时间走的是同一张表的读路径。
	ats, err := repo.UserDeletedAt(ctx, ctype.NewID(200), sessionID, []ctype.ID{m1.ID, m2.ID, m3.ID})
	if err != nil {
		t.Fatalf("查删除时间失败: %v", err)
	}
	if len(ats) != 2 {
		t.Fatalf("应能查到 2 条删除时间，实际 %d", len(ats))
	}
	if _, ok := ats[m3.ID]; ok {
		t.Fatal("未删除的 m3 不该有删除时间")
	}

	// 幂等：再删一次不报错，也不新增状态行（唯一键 + upsert）。
	again, err := repo.DeleteMessagesForUser(ctx, ctype.NewID(200), []ctype.ID{m1.ID})
	if err != nil {
		t.Fatalf("重复删除失败: %v", err)
	}
	if len(again) != 1 {
		t.Fatalf("重复删除应命中 1 条消息，实际 %d", len(again))
	}
	if n := countUserStateRows(t, db, 200, sessionID); n != 2 {
		t.Fatalf("重复删除不该新增状态行，实际 %d", n)
	}

	// 非参与者删不掉：条件必须是 sender_id 或 receiver_id = 该用户。
	none, err := repo.DeleteMessagesForUser(ctx, ctype.NewID(300), []ctype.ID{m1.ID, m2.ID, m3.ID})
	if err != nil {
		t.Fatalf("非参与者删除失败: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("非参与者不该删掉别人的消息，实际命中 %d 条", len(none))
	}
}

// countUserStateRows 数某个用户在某个会话下的软删状态行。
//
// 这里直接查真表而不是只走接口：Ticket #190 要证明的正是"写入落在
// schema.sql 里那张 chat_msg_user_state_models"，而不是一张名字相近的表。
func countUserStateRows(t *testing.T, db *gorm.DB, userID uint64, sessionID string) int64 {
	t.Helper()
	var n int64
	if err := db.Table("chat_msg_user_state_models").
		Where("user_id = ? AND session_id = ? AND deleted_at IS NOT NULL", userID, sessionID).
		Count(&n).Error; err != nil {
		t.Fatalf("统计状态行失败: %v", err)
	}
	return n
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

	rows, total, err := repo.ListSessions(context.Background(), ctype.NewID(100), false, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查会话列表失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("应有 2 条会话，实际 %d", total)
	}
	if len(rows) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(rows))
	}
	if rows[0].ReceiverID.Uint64() != 300 {
		t.Fatalf("最新对话应排第一（receiver=300），实际 receiver=%s", rows[0].ReceiverID)
	}
	_ = db
}

// TestSendWithClientKeyIsIdempotent 同一 client_msg_id 重复提交必须：
//   - 返回第一次落库的那条消息（同一个 ID）；
//   - 消息表只有一条（不是两条）；
//   - 接收方未读只加 1（重复提交整体回滚，不会再加一次）；
//   - 发件箱只有一条通知事件（否则对方会收到两次"新消息"）。
//
// 幂等靠"（发送人, 幂等键）→ 主键"的确定性映射 + 主键唯一约束实现，
// 所以它不依赖 Redis、也不依赖进程内状态。换了幂等键就是一条新消息 ——
// 这条同时钉住"幂等不能退化成内容去重"（同一句话发两次是两条真实消息）。
func TestSendWithClientKeyIsIdempotent(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	first, duplicated := sendTextWithKey(t, repo, 100, 200, "在吗", "client-key-1", base)
	if duplicated {
		t.Fatal("第一次发送不该是幂等命中")
	}

	second, duplicated := sendTextWithKey(t, repo, 100, 200, "在吗", "client-key-1", base.Add(time.Second))
	if !duplicated {
		t.Fatal("重复提交同一个幂等键应命中幂等，而不是产生第二条消息")
	}
	if second.ID != first.ID {
		t.Fatalf("幂等命中应返回第一次的消息 id=%s，实际 %s", first.ID, second.ID)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("幂等命中应返回同一会话 %s，实际 %s", first.SessionID, second.SessionID)
	}

	var msgCount int64
	if err := db.Table("chat_msg_models").Count(&msgCount).Error; err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("重复提交只能有一条消息，实际 %d 条", msgCount)
	}

	unread, _, lastID, _ := sessionOf(t, db, 200, domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200)))
	if unread != 1 {
		t.Fatalf("重复提交不该把未读再加一次，实际未读 %d", unread)
	}
	if lastID != first.ID.Uint64() {
		t.Fatalf("会话最后一条消息应仍是幂等命中那条 id=%s，实际 %d", first.ID, lastID)
	}
	if n := len(outboxEvents(t, db, domain.EventMessageSent)); n != 1 {
		t.Fatalf("重复提交不该多写一条发送通知事件，实际 %d 条", n)
	}

	// 换一个幂等键就是一条新消息（不是内容去重）。
	third, duplicated := sendTextWithKey(t, repo, 100, 200, "在吗", "client-key-2", base.Add(2*time.Second))
	if duplicated {
		t.Fatal("不同幂等键不该被当成重复提交")
	}
	if third.ID == first.ID {
		t.Fatal("不同幂等键应得到不同消息 ID")
	}
	if err := db.Table("chat_msg_models").Count(&msgCount).Error; err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	if msgCount != 2 {
		t.Fatalf("不同幂等键应产生第二条消息，实际 %d 条", msgCount)
	}
	unread, _, _, _ = sessionOf(t, db, 200, domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200)))
	if unread != 2 {
		t.Fatalf("第二条真实消息应把未读加到 2，实际 %d", unread)
	}
}

// TestSendWritesOutboxEventInSameTransaction 发送必须在同一事务里写一条
// chat.message.sent 发件箱事件：这是"WS 降级为通知通道"的投递来源。
//
// 断言事件本身（名字、聚合、状态、载荷）而不是"订阅者注册了"：
// 只有事件真的落表，中继才可能在崩溃后重投、对方不在线时才可能在下一轮补推。
func TestSendWritesOutboxEventInSameTransaction(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	msg, _ := sendText(t, repo, 100, 200, "在吗", base)

	events := outboxEvents(t, db, domain.EventMessageSent)
	if len(events) != 1 {
		t.Fatalf("发送应写 1 条 %s 事件，实际 %d 条", domain.EventMessageSent, len(events))
	}
	event := events[0]
	if event.AggregateType != domain.OutboxAggregateType {
		t.Fatalf("聚合类型应为 %s，实际 %s", domain.OutboxAggregateType, event.AggregateType)
	}
	if event.AggregateID != msg.ID {
		t.Fatalf("事件聚合 ID 应为消息 ID %s，实际 %s", msg.ID, event.AggregateID)
	}
	if event.Status != schema.OutboxStatusPending {
		t.Fatalf("事件初始状态应为 pending，实际 %s", event.Status)
	}

	var notice domain.SentNotice
	if err := json.Unmarshal([]byte(event.Payload), &notice); err != nil {
		t.Fatalf("事件载荷不是合法的 SentNotice: %v", err)
	}
	if notice.MessageID != msg.ID || notice.SessionID != msg.SessionID {
		t.Fatalf("事件载荷与消息不一致: %+v", notice)
	}
	if notice.SenderID.Uint64() != 100 || notice.ReceiverID.Uint64() != 200 {
		t.Fatalf("事件载荷收发人不正确: sender=%s receiver=%s", notice.SenderID, notice.ReceiverID)
	}
	// 通知不带正文：正文由接收方收到通知后走 HTTP 重新拉取。
	if strings.Contains(event.Payload, "在吗") {
		t.Fatalf("发件箱事件不该带上消息正文，实际载荷 %s", event.Payload)
	}
}

// TestMarkReadWritesOutboxReadEvent 已读回执也走发件箱：
// 按（发送人, 会话）聚合成一条 chat.message.read 事件，由中继推给发送方。
//
// 不直推连接池的意义：标记已读是写操作，崩溃/断线后回执仍要能补发。
func TestMarkReadWritesOutboxReadEvent(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m1, _ := sendText(t, repo, 100, 200, "一", base)
	m2, _ := sendText(t, repo, 100, 200, "二", base.Add(time.Minute))

	affected, err := repo.MarkRead(context.Background(), ctype.NewID(200), []ctype.ID{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	if len(affected) != 2 {
		t.Fatalf("应命中 2 条未读消息，实际 %d", len(affected))
	}

	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))
	events := outboxEvents(t, db, domain.EventMessagesRead)
	if len(events) != 1 {
		t.Fatalf("同一发送人+会话应聚合成 1 条已读事件，实际 %d 条", len(events))
	}

	var read domain.ReadEvent
	if err := json.Unmarshal([]byte(events[0].Payload), &read); err != nil {
		t.Fatalf("事件载荷不是合法的 ReadEvent: %v", err)
	}
	if read.ReaderID.Uint64() != 200 {
		t.Fatalf("已读事件读者应为 200，实际 %s", read.ReaderID)
	}
	if read.SenderID.Uint64() != 100 {
		t.Fatalf("已读事件应推给发送人 100，实际 %s", read.SenderID)
	}
	if read.SessionID != sessionID {
		t.Fatalf("已读事件会话不正确，实际 %s", read.SessionID)
	}
	if len(read.MsgIDList) != 2 {
		t.Fatalf("已读事件应带上本次命中的 2 条消息，实际 %d", len(read.MsgIDList))
	}
}

// outboxRow 是发件箱事件在测试里的读模型。
type outboxRow struct {
	EventName     string
	AggregateType string
	AggregateID   ctype.ID
	Payload       string
	Status        string
}

// outboxEvents 按事件名读发件箱里的行。
func outboxEvents(t *testing.T, db *gorm.DB, name string) []outboxRow {
	t.Helper()
	var rows []outboxRow
	if err := db.Table("outbox_event_models").
		Select("event_name", "aggregate_type", "aggregate_id", "payload", "status").
		Where("event_name = ?", name).
		Order("id asc").
		Find(&rows).Error; err != nil {
		t.Fatalf("读发件箱事件失败: %v", err)
	}
	return rows
}

// pickNonMonotonicClientKeys 挑两个幂等键：前一个派生出的主键更大、后一个更小。
//
// 客户端幂等键派生的是 [2^62,2^63) 区间的 FNV 哈希，主键大小与消息先后无关；
// 普通雪花主键远小于这个区间，所以「时间最晚」和「MAX(id)」在这里必然是两条。
func pickNonMonotonicClientKeys(sender ctype.ID) (bigger, smaller string) {
	type candidate struct {
		key string
		id  uint64
	}
	var list []candidate
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("278-clear-key-%d", i)
		list = append(list, candidate{key: key, id: domain.MessageIDFromClientKey(sender, key).Uint64()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id > list[j].id })
	return list[0].key, list[len(list)-1].key
}

// TestClearWatermarkAdvancesBySendTimeUnderNonMonotonicIDs 清空水位按消息时间推进。
//
// 破口在生产者：清空时用 MAX(id) 挑水位行，而幂等键派生的主键非单调，
// 于是挑到一条时间上更早的消息 —— 清空后旧历史重现、未读被并账复活，
// 消息列表 / 整会话已读 / 消息中心角标 / 30 分钟并账四者说法分裂。
// 这里全部走公开仓储接口与公开读取器，不越过接口测私有 helper。
func TestClearWatermarkAdvancesBySendTimeUnderNonMonotonicIDs(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sender := ctype.NewID(100)
	receiver := ctype.NewID(200)
	sessionID := domain.BuildSessionID(sender, receiver)

	bigKey, smallKey := pickNonMonotonicClientKeys(sender)
	oldHigh, _ := sendTextWithKey(t, repo, 100, 200, "old-high", bigKey, base)
	// 会话可见集含自己发的消息：这一条是接收者 200 发给 100 的。
	sendText(t, repo, 200, 100, "self-sent", base.Add(time.Minute))
	// 时间上最晚的清空前消息，主键反而比 old-high 小。
	newestBeforeClear, _ := sendTextWithKey(t, repo, 100, 200, "newest-before-clear", smallKey, base.Add(2*time.Minute))
	if oldHigh.ID.Uint64() <= newestBeforeClear.ID.Uint64() {
		t.Fatalf("夹具要求非单调主键: old=%s newest=%s", oldHigh.ID, newestBeforeClear.ID)
	}

	deleted, err := repo.DeleteSessions(ctx, receiver, []string{sessionID})
	if err != nil {
		t.Fatalf("清空会话失败: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != sessionID {
		t.Fatalf("应清空 1 条会话 %s，实际 %v", sessionID, deleted)
	}

	// 清空水位是「清到哪条消息」；用例层从会话行读出来，再喂给消息列表。
	clearedSession, err := repo.FindSession(ctx, sessionID, receiver, true)
	if err != nil {
		t.Fatalf("读清空后的会话失败: %v", err)
	}
	if clearedSession.ClearBeforeMsgID != newestBeforeClear.ID {
		t.Fatalf("清空水位必须落在时间上最晚那条 %s，实际 %s（MAX(id) 会挑中更早的 %s）",
			newestBeforeClear.ID, clearedSession.ClearBeforeMsgID, oldHigh.ID)
	}

	// 对方再来一条消息：软删的会话恢复，清空水位保留。
	after, _ := sendText(t, repo, 100, 200, "after-clear", base.Add(3*time.Minute))

	visible, err := repo.ListMessages(ctx, domain.MessageQuery{
		UserID:      receiver,
		SessionID:   sessionID,
		ClearBefore: clearedSession.ClearBeforeMsgID,
		Page:        domain.Page{Page: 1, Limit: 10},
	})
	if err != nil {
		t.Fatalf("清空后查消息列表失败: %v", err)
	}
	if visible.Total != 1 || len(visible.Messages) != 1 || visible.Messages[0].ID != after.ID {
		t.Fatalf("清空后旧历史不该重现，可见集应只有 after-clear=%s，实际 total=%d len=%d messages=%+v",
			after.ID, visible.Total, len(visible.Messages), visible.Messages)
	}

	badgeRepo := notifmysql.NewChatUnread(db)
	badge, err := badgeRepo.UnreadTotal(ctx, 200)
	if err != nil {
		t.Fatalf("读消息中心角标失败: %v", err)
	}
	if badge != 1 {
		t.Fatalf("清空后角标只应数 after-clear 一条未读，实际 %d", badge)
	}

	// 30 分钟并账（公开入口）：按同一份可见性重算，不能把清掉的未读复活。
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	badge, err = badgeRepo.UnreadTotal(ctx, 200)
	if err != nil {
		t.Fatalf("并账后读角标失败: %v", err)
	}
	if badge != 1 {
		t.Fatalf("并账后角标应仍是 1，实际 %d（清掉的未读被复活了）", badge)
	}

	// 整会话已读只应命中水位之后、发给我的那一条。
	marked, err := repo.MarkReadSession(ctx, receiver, sessionID)
	if err != nil {
		t.Fatalf("整会话已读失败: %v", err)
	}
	if len(marked) != 1 || marked[0].ID != after.ID {
		t.Fatalf("整会话已读应只命中 after-clear=%s，实际 %+v", after.ID, marked)
	}
	badge, err = badgeRepo.UnreadTotal(ctx, 200)
	if err != nil {
		t.Fatalf("已读后读角标失败: %v", err)
	}
	if badge != 0 {
		t.Fatalf("整会话已读后角标应归零，实际 %d", badge)
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("已读后并账失败: %v", err)
	}
	badge, err = badgeRepo.UnreadTotal(ctx, 200)
	if err != nil {
		t.Fatalf("已读后并账读角标失败: %v", err)
	}
	if badge != 0 {
		t.Fatalf("已读后并账仍应给 0，实际 %d（四者说法不一致）", badge)
	}
}
