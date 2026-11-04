package mysql

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// 本文件守「未读是可重算的派生值」这条口径（spec 269 / D-2，票 266）：
//
//  1. 漂移能自愈：库里的 unread_count 被改脏，重算后等于消息行的真值；
//  2. 只数发给我的、且未读的消息；
//  3. clear_before_msg_id 对应 send_time 处及之前的消息不参与（清除会话后未读不复活）；
//  4. 用户级删除的消息不参与；
//  5. 软删的会话不写、也不报错；
//  6. 全量入口按游标分页跑完所有会话。
//
// 与 chat_integration_test.go 的分工：这里用 sqlite 验证"算得对"，
// 真实外键 / affected rows 口径由 MySQL 容器用例守。

func newUnreadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.SetupSQLite(t, &sessionRow{}, &msgRow{}, &stateRow{})
}

// seedSession 造一条活跃会话。unread 故意传一个漂移值，验证覆盖写。
func seedSession(t *testing.T, db *gorm.DB, id uint64, sessionID string, userID, receiverID uint64, unread int, clearBefore uint64) sessionRow {
	t.Helper()
	row := sessionRow{
		ID:               ctype.NewID(id),
		SessionID:        sessionID,
		UserID:           ctype.NewID(userID),
		ReceiverID:       ctype.NewID(receiverID),
		ClearBeforeMsgID: ctype.NewID(clearBefore),
		UnreadCount:      unread,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	return row
}

func seedMsg(t *testing.T, db *gorm.DB, id uint64, sessionID string, senderID, receiverID uint64, status domain.MsgStatus) {
	t.Helper()
	seedMsgAt(t, db, ctype.NewID(id), sessionID, ctype.NewID(senderID), ctype.NewID(receiverID), status, time.Now())
}

// seedMsgAt 造一条带**明确 send_time**的消息。
//
// 清空水位按 send_time 算而不是 id，所以测试必须能控制时间轴；用 time.Now()
// 会让同一批消息的先后变得不可判定（毫秒精度下可能完全相同）。
func seedMsgAt(t *testing.T, db *gorm.DB, id ctype.ID, sessionID string, senderID, receiverID ctype.ID, status domain.MsgStatus, at time.Time) {
	t.Helper()
	row := msgRow{
		ID:         id,
		SessionID:  sessionID,
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    "hello",
		SendTime:   at,
		MsgStatus:  int8(status),
		MsgType:    int8(domain.TypeText),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建消息 %s 失败: %v", id, err)
	}
}

func mustUnread(t *testing.T, db *gorm.DB, id uint64) int {
	t.Helper()
	var row sessionRow
	if err := db.Unscoped().Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("读会话失败: %v", err)
	}
	return row.UnreadCount
}

func TestReconcileChatUnreadRecomputesFromMessageRows(t *testing.T) {
	db := newUnreadTestDB(t)
	seedSession(t, db, 1, "chat:100:200", 100, 200, 9, 0) // 漂移成 9

	// 真值：3 条发给我且未读，1 条已读，1 条发给别人。
	seedMsg(t, db, 11, "chat:100:200", 200, 100, domain.StatusSent)
	seedMsg(t, db, 12, "chat:100:200", 200, 100, domain.StatusSent)
	seedMsg(t, db, 13, "chat:100:200", 200, 100, domain.StatusSent)
	seedMsg(t, db, 14, "chat:100:200", 200, 100, domain.StatusRead)
	seedMsg(t, db, 15, "chat:100:200", 100, 200, domain.StatusSent)

	hit, err := ReconcileChatUnread(context.Background(), db, "chat:100:200", 100)
	if err != nil || !hit {
		t.Fatalf("重算应命中: hit=%v err=%v", hit, err)
	}
	if got := mustUnread(t, db, 1); got != 3 {
		t.Fatalf("未读应按消息行重算为 3（漂移 9 必须被覆盖），实际 %d", got)
	}
}

func TestReconcileChatUnreadRespectsClearBefore(t *testing.T) {
	db := newUnreadTestDB(t)
	// 清除水位 = 那条 send_time=10:01 的消息（它的 ID 是个 [2^62,2^63) 的哈希式大数）。
	//
	// 旧口径 `id > clear_before_msg_id` 在这里会全错：m-old 的 ID 比水位小、
	// m-new 的 ID 也比水位小，一条都不该按 ID 判。真值是"只数 send_time 严格
	// 晚于水位行的消息"，所以只有 m-new 进未读。
	cleared := domain.MessageIDFromClientKey(ctype.NewID(200), "clear-key")
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	seedSession(t, db, 2, "chat:100:200", 100, 200, 0, cleared.Uint64())
	seedMsgAt(t, db, ctype.NewID(11), "chat:100:200", ctype.NewID(200), ctype.NewID(100), domain.StatusSent, at)
	seedMsgAt(t, db, cleared, "chat:100:200", ctype.NewID(200), ctype.NewID(100), domain.StatusSent, at.Add(time.Minute))
	seedMsgAt(t, db, ctype.NewID(13), "chat:100:200", ctype.NewID(200), ctype.NewID(100), domain.StatusSent, at.Add(2*time.Minute))

	if _, err := ReconcileChatUnread(context.Background(), db, "chat:100:200", 100); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	if got := mustUnread(t, db, 2); got != 1 {
		t.Fatalf("清除水位处及之前的消息不得计入未读，期望 1，实际 %d", got)
	}
}

// TestReconcileChatUnreadClearWatermarkSurvivesSoftDeletedRow 水位行被软删后
// 仍能解析出 send_time（Unscoped 子查询），清空的历史不会在重算时复活。
func TestReconcileChatUnreadClearWatermarkSurvivesSoftDeletedRow(t *testing.T) {
	db := newUnreadTestDB(t)
	cleared := domain.MessageIDFromClientKey(ctype.NewID(200), "clear-key-2")
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	seedSession(t, db, 8, "chat:100:200", 100, 200, 9, cleared.Uint64())
	seedMsgAt(t, db, cleared, "chat:100:200", ctype.NewID(200), ctype.NewID(100), domain.StatusSent, at)
	seedMsgAt(t, db, ctype.NewID(21), "chat:100:200", ctype.NewID(200), ctype.NewID(100), domain.StatusSent, at.Add(time.Minute))
	if err := db.Delete(&msgRow{}, "id = ?", cleared).Error; err != nil {
		t.Fatalf("软删水位消息失败: %v", err)
	}

	if _, err := ReconcileChatUnread(context.Background(), db, "chat:100:200", 100); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	if got := mustUnread(t, db, 8); got != 1 {
		t.Fatalf("水位行软删后仍应只剩 1 条未读，实际 %d（子查询若被默认范围挡掉会复活历史）", got)
	}
}

func TestReconcileChatUnreadExcludesUserDeletedMessages(t *testing.T) {
	db := newUnreadTestDB(t)
	seedSession(t, db, 3, "chat:100:200", 100, 200, 5, 0)
	seedMsg(t, db, 11, "chat:100:200", 200, 100, domain.StatusSent)
	seedMsg(t, db, 12, "chat:100:200", 200, 100, domain.StatusSent)

	// 用户 100 删掉了第 12 条（软删的用户级状态行）。
	state := stateRow{
		ID:        ctype.NewID(31),
		MsgID:     ctype.NewID(12),
		UserID:    ctype.NewID(100),
		SessionID: "chat:100:200",
		DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("创建用户级删除状态失败: %v", err)
	}

	if _, err := ReconcileChatUnread(context.Background(), db, "chat:100:200", 100); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	if got := mustUnread(t, db, 3); got != 1 {
		t.Fatalf("已删除的消息不得计入未读，期望 1，实际 %d", got)
	}
}

func TestReconcileChatUnreadSkipsSoftDeletedSession(t *testing.T) {
	db := newUnreadTestDB(t)
	seedSession(t, db, 4, "chat:100:200", 100, 200, 9, 0)
	seedMsg(t, db, 11, "chat:100:200", 200, 100, domain.StatusSent)
	if err := db.Delete(&sessionRow{}, "id = ?", 4).Error; err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}

	hit, err := ReconcileChatUnread(context.Background(), db, "chat:100:200", 100)
	if err != nil {
		t.Fatalf("软删会话不该报错: %v", err)
	}
	if hit {
		t.Fatalf("软删会话不该被写回")
	}
}

func TestReconcileAllChatUnreadPaginatesAllSessions(t *testing.T) {
	db := newUnreadTestDB(t)
	// batch=1，强制走多轮游标；三条会话都要被覆盖。
	seedSession(t, db, 5, "chat:1:2", 1, 2, 7, 0)
	seedSession(t, db, 6, "chat:3:4", 3, 4, 7, 0)
	seedSession(t, db, 7, "chat:5:6", 5, 6, 7, 0)
	seedMsg(t, db, 21, "chat:1:2", 2, 1, domain.StatusSent)
	seedMsg(t, db, 22, "chat:3:4", 4, 3, domain.StatusSent)
	seedMsg(t, db, 23, "chat:3:4", 4, 3, domain.StatusSent)

	total, err := ReconcileAllChatUnread(context.Background(), db, 1)
	if err != nil {
		t.Fatalf("全量重算失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("应写回 3 条会话，实际 %d", total)
	}
	if got := mustUnread(t, db, 5); got != 1 {
		t.Fatalf("会话 5 未读应为 1，实际 %d", got)
	}
	if got := mustUnread(t, db, 6); got != 2 {
		t.Fatalf("会话 6 未读应为 2，实际 %d", got)
	}
	if got := mustUnread(t, db, 7); got != 0 {
		t.Fatalf("会话 7 未读应为 0，实际 %d", got)
	}
}
