//go:build integration

package mysql_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"myblogx/internal/chat/adapter/mysql"
	"myblogx/internal/chat/domain"
	"myblogx/internal/chat/domain/msgid"
	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

// pickNonMonotonicClientKeys 挑两个幂等键：前一个派生出的主键更大、后一个更小。
//
// 幂等键派生出的主键大小与消息先后无关（派生规则见 msgid 包，号段是它的
// 实现细节）；普通雪花主键远小于这个号段，所以「时间最晚」和「MAX(id)」
// 在这里必然是两条。
func pickNonMonotonicClientKeys(sender ctype.ID) (bigger, smaller string) {
	type candidate struct {
		key string
		id  uint64
	}
	var list []candidate
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("278-clear-key-%d", i)
		list = append(list, candidate{key: key, id: msgid.FromClientKey(sender, key).Uint64()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id > list[j].id })
	return list[0].key, list[len(list)-1].key
}

// privateBadge 读消息中心角标的私信数字。
//
// 这是通知模块 ChatUnread.UnreadTotal 的公开聚合口径：SUM(会话行 unread_count)。
// 测试活在 chat 模块，跨模块 import 别人的 adapter 会破模块边界（archcheck R11），
// 因此按同一句公开 SQL 观察结果；验的是四者给同一答案，不是单测任何私有 helper。
func privateBadge(t *testing.T, db *gorm.DB, userID uint64) int64 {
	t.Helper()
	var total int64
	if err := db.Table("chat_session_models").
		Select("COALESCE(SUM(unread_count), 0)").
		Where("user_id = ? AND unread_count > 0 AND deleted_at IS NULL", userID).
		Scan(&total).Error; err != nil {
		t.Fatalf("读消息中心角标失败: %v", err)
	}
	return total
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

	badge := privateBadge(t, db, 200)
	if badge != 1 {
		t.Fatalf("清空后角标只应数 after-clear 一条未读，实际 %d", badge)
	}

	// 30 分钟并账（公开入口）：按同一份可见性重算，不能把清掉的未读复活。
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	badge = privateBadge(t, db, 200)
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
	badge = privateBadge(t, db, 200)
	if badge != 0 {
		t.Fatalf("整会话已读后角标应归零，实际 %d", badge)
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("已读后并账失败: %v", err)
	}
	badge = privateBadge(t, db, 200)
	if badge != 0 {
		t.Fatalf("已读后并账仍应给 0，实际 %d（四者说法不一致）", badge)
	}
}

// visibleContents 走消息列表用例层的公开形状：从会话行取水位再查列表。
func visibleContents(t *testing.T, repo *mysql.Repo, userID ctype.ID, sessionID string) []string {
	t.Helper()
	session, err := repo.FindSession(context.Background(), sessionID, userID, false)
	if err != nil {
		t.Fatalf("读会话失败: %v", err)
	}
	got, err := repo.ListMessages(context.Background(), domain.MessageQuery{
		UserID:      userID,
		SessionID:   sessionID,
		ClearBefore: session.ClearBeforeMsgID,
		Page:        domain.Page{Page: 1, Limit: 50},
	})
	if err != nil {
		t.Fatalf("查消息列表失败: %v", err)
	}
	out := make([]string, 0, len(got.Messages))
	for _, msg := range got.Messages {
		out = append(out, msg.Content)
	}
	sort.Strings(out)
	return out
}

// assertVisibleAndUnread 在同一组数据上验四者说法一致。
//
// ① 可见集含自己发的消息；② 未读只数「接收者是我且未读」；
// ③ 会话 badge / 消息中心角标读同一派生列；④ 30 分钟并账覆盖写回同一真值。
func assertVisibleAndUnread(t *testing.T, repo *mysql.Repo, db *gorm.DB, userID ctype.ID, sessionID string, wantContents []string, wantUnread int) {
	t.Helper()
	ctx := context.Background()
	got := visibleContents(t, repo, userID, sessionID)
	want := append([]string(nil), wantContents...)
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("可见集不符: got=%v want=%v", got, want)
	}

	// 整会话已读：只应命中「发给 userID 且未读」的那些消息。
	marked, err := repo.MarkReadSession(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("整会话已读失败: %v", err)
	}
	if len(marked) != wantUnread {
		t.Fatalf("整会话已读应命中 %d 条（只数发给我的未读），实际 %d", wantUnread, len(marked))
	}
	for _, msg := range marked {
		if msg.ReceiverID != userID {
			t.Fatalf("整会话已读命中了不是发给我的消息: %+v", msg)
		}
	}

	// 已读后会话 badge 与消息中心角标都必须是 0；并账也不该把它们改回去。
	rows, _, err := repo.ListSessions(ctx, domain.SessionQuery{UserID: userID, Page: domain.Page{Page: 1, Limit: 10}})
	if err != nil {
		t.Fatalf("查会话列表失败: %v", err)
	}
	for _, row := range rows {
		if row.SessionID == sessionID && row.UnreadCount != 0 {
			t.Fatalf("会话 badge 应归零，实际 %d", row.UnreadCount)
		}
	}
	if badge := privateBadge(t, db, userID.Uint64()); badge != 0 {
		t.Fatalf("消息中心角标应归零，实际 %d", badge)
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, userID.Uint64()); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	if badge := privateBadge(t, db, userID.Uint64()); badge != 0 {
		t.Fatalf("已读后并账仍应给 0，实际 %d", badge)
	}
}

// TestReconcileAllChatUnreadPaginatesAllSessions 全量并账公开入口（批量游标）。
//
// 同一批里有多对用户、多个会话，且故意把派生列写脏；一轮并账后每条会话
// 都覆盖回消息行真值，消息中心角标等于这些真值之和。
func TestReconcileAllChatUnreadPaginatesAllSessions(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	sendText(t, repo, 100, 200, "a-1", base)
	sendText(t, repo, 100, 200, "a-2", base.Add(time.Minute))
	sendText(t, repo, 100, 300, "b-1", base.Add(2*time.Minute))
	sendText(t, repo, 300, 200, "c-1", base.Add(3*time.Minute))

	// 把所有会话行的派生列写脏，验证覆盖写而不是 += / -= 增量。
	if err := db.Exec("UPDATE chat_session_models SET unread_count = 99").Error; err != nil {
		t.Fatalf("写脏派生列失败: %v", err)
	}
	if total, err := mysql.ReconcileAllChatUnread(ctx, db, 1); err != nil {
		t.Fatalf("全量并账失败: %v", err)
	} else if total == 0 {
		t.Fatal("全量并账应至少写回一条会话")
	}

	// 逐用户验：角标 = 该用户所有会话里「发给自己且未读」的消息行数之和。
	for _, userID := range []uint64{100, 200, 300} {
		var truth int64
		if err := db.Table("chat_msg_models").
			Where("receiver_id = ? AND msg_status < ? AND deleted_at IS NULL", userID, int8(domain.StatusRead)).
			Count(&truth).Error; err != nil {
			t.Fatalf("数用户 %d 的真值失败: %v", userID, err)
		}
		if badge := privateBadge(t, db, userID); badge != truth {
			t.Fatalf("用户 %d 角标与并账真值不一致: badge=%d truth=%d", userID, badge, truth)
		}
	}
}

// TestVisibilitySetIncludesOwnMessagesButUnreadOnlyCountsIncoming 两类集合边界。
//
// 可见集含自己发的消息（不看 sender_id / receiver_id）；未读数只数
// 「接收者是我且未读」。会话列表、整会话已读、消息中心角标、并账同组数据同答案。
func TestVisibilitySetIncludesOwnMessagesButUnreadOnlyCountsIncoming(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sessionID := domain.BuildSessionID(ctype.NewID(100), ctype.NewID(200))

	sendText(t, repo, 100, 200, "incoming-1", base)
	sendText(t, repo, 100, 200, "incoming-2", base.Add(time.Minute))
	sendText(t, repo, 200, 100, "outgoing-1", base.Add(2*time.Minute))
	sendText(t, repo, 100, 300, "other-session", base.Add(3*time.Minute))

	// 接收者 200 的可见集：3 条（含自己发的 outgoing-1），不含别的会话。
	assertVisibleAndUnread(t, repo, db, ctype.NewID(200), sessionID,
		[]string{"incoming-1", "incoming-2", "outgoing-1"}, 2)
}

// TestVisibilityRespectsUserDeletesAndSoftDeletedWatermarkRow 用户级软删 +
// 水位行被软删：列表 / 已读 / 角标 / 并账都不复活被清掉的消息。
func TestVisibilityRespectsUserDeletesAndSoftDeletedWatermarkRow(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	receiver := ctype.NewID(200)
	sessionID := domain.BuildSessionID(ctype.NewID(100), receiver)

	old, _ := sendText(t, repo, 100, 200, "old", base)
	watermark, _ := sendText(t, repo, 100, 200, "watermark", base.Add(time.Minute))
	_ = old

	if _, err := repo.DeleteSessions(ctx, receiver, []string{sessionID}); err != nil {
		t.Fatalf("清空会话失败: %v", err)
	}
	// 水位行本身被用户级软删：Unscoped 子查询仍要能解出它的 send_time。
	if err := db.Table("chat_msg_models").
		Where("id = ?", watermark.ID.Uint64()).
		Update("deleted_at", base.Add(2*time.Minute)).Error; err != nil {
		t.Fatalf("软删水位行失败: %v", err)
	}
	cleared, err := repo.FindSession(ctx, sessionID, receiver, true)
	if err != nil {
		t.Fatalf("读清空后的会话失败: %v", err)
	}
	if cleared.ClearBeforeMsgID != watermark.ID {
		t.Fatalf("水位应落在时间最晚的 watermark=%s，实际 %s", watermark.ID, cleared.ClearBeforeMsgID)
	}

	// 清空后新消息里再软删一条：水位与用户级软删两个条件各自独立生效。
	fresh, _ := sendText(t, repo, 100, 200, "fresh", base.Add(3*time.Minute))
	deleted, _ := sendText(t, repo, 100, 200, "user-deleted", base.Add(4*time.Minute))
	revive, _ := sendText(t, repo, 100, 200, "revive", base.Add(5*time.Minute))
	if _, err := repo.DeleteMessagesForUser(ctx, receiver, []ctype.ID{deleted.ID}); err != nil {
		t.Fatalf("用户级删除失败: %v", err)
	}
	_ = fresh
	_ = revive

	got := visibleContents(t, repo, receiver, sessionID)
	if strings.Join(got, "|") != "fresh|revive" {
		t.Fatalf("清空 + 水位行软删 + 用户级软删后可见集应只剩 fresh/revive，实际 %v", got)
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	if badge := privateBadge(t, db, 200); badge != 2 {
		t.Fatalf("角标/并账应只数 fresh+revive 两条（被软删的不计），实际 %d", badge)
	}
	marked, err := repo.MarkReadSession(ctx, receiver, sessionID)
	if err != nil {
		t.Fatalf("整会话已读失败: %v", err)
	}
	if len(marked) != 2 {
		t.Fatalf("整会话已读应命中 2 条（被软删的不计），实际 %d", len(marked))
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("已读后并账失败: %v", err)
	}
	if badge := privateBadge(t, db, 200); badge != 0 {
		t.Fatalf("已读后并账应给 0，实际 %d", badge)
	}
}

// TestClearWatermarkUsesSameMillisecondTieBreak 同毫秒并列：水位按
// (send_time, id) 确定性裁决，清空后并列历史不重现、未读不复活。
func TestClearWatermarkUsesSameMillisecondTieBreak(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	receiver := ctype.NewID(200)
	sessionID := domain.BuildSessionID(ctype.NewID(100), receiver)

	firstKey, secondKey := pickNonMonotonicClientKeys(ctype.NewID(100))
	first, _ := sendTextWithKey(t, repo, 100, 200, "same-ms-1", firstKey, at)
	second, _ := sendTextWithKey(t, repo, 100, 200, "same-ms-2", secondKey, at)
	// 挑出并列里 (send_time, id) 更晚的那条作为期望水位。
	wantWatermark := first.ID
	if second.ID.Uint64() > first.ID.Uint64() {
		wantWatermark = second.ID
	}

	if _, err := repo.DeleteSessions(ctx, receiver, []string{sessionID}); err != nil {
		t.Fatalf("清空会话失败: %v", err)
	}
	cleared, err := repo.FindSession(ctx, sessionID, receiver, true)
	if err != nil {
		t.Fatalf("读清空后的会话失败: %v", err)
	}
	if cleared.ClearBeforeMsgID != wantWatermark {
		t.Fatalf("同毫秒并列水位应是 %s，实际 %s", wantWatermark, cleared.ClearBeforeMsgID)
	}
	sendText(t, repo, 100, 200, "after", at.Add(time.Second))
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	if badge := privateBadge(t, db, 200); badge != 1 {
		t.Fatalf("并列历史不该复活未读，角标应为 1，实际 %d", badge)
	}
	if got := visibleContents(t, repo, receiver, sessionID); strings.Join(got, "|") != "after" {
		t.Fatalf("并列历史不该重现，可见集应只有 after，实际 %v", got)
	}
}

// TestConcurrentSendAndReadStayConsistentAfterReconcile 并发发送与整会话已读交错。
//
// 两条发送与一次已读同时发生；并账必须把会话 badge / 消息中心角标
// 覆盖回消息行真值，不留下永久漂移。
func TestConcurrentSendAndReadStayConsistentAfterReconcile(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	receiver := ctype.NewID(200)
	sessionID := domain.BuildSessionID(ctype.NewID(100), receiver)

	// 先发两条，保证并发阶段既有可读存量又有新增。
	sendText(t, repo, 100, 200, "seed-1", base)
	sendText(t, repo, 100, 200, "seed-2", base.Add(time.Minute))

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, _, err := repo.Send(ctx, domain.SendCommand{
			SenderID: ctype.NewID(100), ReceiverID: receiver, Content: "concurrent-1",
			SendTime: base.Add(2 * time.Minute), MsgType: domain.TypeText,
		}, domain.Display{Nickname: "甲"}, domain.Display{Nickname: "乙"}, "concurrent-1")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, _, err := repo.Send(ctx, domain.SendCommand{
			SenderID: ctype.NewID(100), ReceiverID: receiver, Content: "concurrent-2",
			SendTime: base.Add(3 * time.Minute), MsgType: domain.TypeText,
		}, domain.Display{Nickname: "甲"}, domain.Display{Nickname: "乙"}, "concurrent-2")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := repo.MarkReadSession(ctx, receiver, sessionID)
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发发送/已读失败: %v", err)
		}
	}

	// 真值 = 所有「发给我且未读」的消息行数；并账后 badge / 角标都等于它。
	var truth int64
	if err := db.Table("chat_msg_models").
		Where("session_id = ? AND receiver_id = ? AND msg_status < ? AND deleted_at IS NULL",
			sessionID, 200, int8(domain.StatusRead)).
		Count(&truth).Error; err != nil {
		t.Fatalf("数真值失败: %v", err)
	}
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	rows, _, err := repo.ListSessions(ctx, domain.SessionQuery{UserID: receiver, Page: domain.Page{Page: 1, Limit: 10}})
	if err != nil {
		t.Fatalf("查会话列表失败: %v", err)
	}
	for _, row := range rows {
		if row.SessionID == sessionID && int64(row.UnreadCount) != truth {
			t.Fatalf("会话 badge 与消息行真值不一致: badge=%d truth=%d", row.UnreadCount, truth)
		}
	}
	if badge := privateBadge(t, db, 200); badge != truth {
		t.Fatalf("消息中心角标与并账真值不一致: badge=%d truth=%d", badge, truth)
	}
}

// TestConcurrentSendAndMarkReadSessionDoesNotDeadlock 并发"发送 + 整会话已读"
// 不允许出现 InnoDB 死锁（1213）。
//
// 这是加锁顺序倒置的回归守卫：发送与已读都会先后写 chat_msg_models 与
// chat_session_models。只要一侧写成「会话→消息」、另一侧是「消息→会话」，
// 环就成立（发送持会话行等消息行、已读持消息行等会话行）。修法只能是
// 统一顺序，**不能**是捕获 1213 重试（治理 D10）——所以这里把任何一次 1213
// 都判为失败，而不是"偶发抖动放过"。
//
// 为什么是"1 个发送者 + 2 个已读者、多轮"而不是"一堆发送者"：多发送者
// 同时打同一条会话行还会撞上 ON DUPLICATE KEY UPDATE 的 S→X 升级窗口，
// 那是另一条与本题无关的既有竞态，混进来只会让回归用例本身抖。这里刻意
// 只保留单写者，把竞态收敛到"发送 vs 已读"这唯一一条加锁顺序上。
func TestConcurrentSendAndMarkReadSessionDoesNotDeadlock(t *testing.T) {
	t.Parallel()

	repo, db := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	receiver := ctype.NewID(200)
	sessionID := domain.BuildSessionID(ctype.NewID(100), receiver)
	sendText(t, repo, 100, 200, "seed", base)

	const rounds = 12
	for round := 0; round < rounds; round++ {
		at := base.Add(time.Duration(round+1) * time.Minute)
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := repo.Send(ctx, domain.SendCommand{
				SenderID:   ctype.NewID(100),
				ReceiverID: receiver,
				Content:    fmt.Sprintf("race-%d", round),
				SendTime:   at,
				MsgType:    domain.TypeText,
			}, domain.Display{Nickname: "甲"}, domain.Display{Nickname: "乙"}, "race")
			errs <- err
		}()
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.MarkReadSession(ctx, receiver, sessionID)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("第 %d 轮并发发送/已读失败（死锁 1213 也算失败）: %v", round, err)
			}
		}
	}

	// 没有丢更新：并账仍能把派生列覆盖回消息行真值。
	if _, err := mysql.ReconcileChatUnread(ctx, db, sessionID, 200); err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	var truth int64
	if err := db.Table("chat_msg_models").
		Where("session_id = ? AND receiver_id = ? AND msg_status < ? AND deleted_at IS NULL",
			sessionID, 200, int8(domain.StatusRead)).
		Count(&truth).Error; err != nil {
		t.Fatalf("数真值失败: %v", err)
	}
	if badge := privateBadge(t, db, 200); badge != truth {
		t.Fatalf("并发后角标与消息行真值不一致: badge=%d truth=%d", badge, truth)
	}
}
