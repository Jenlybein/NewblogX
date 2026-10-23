package chat_service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/chat_msg_enum"
	"myblogx/test/testutil"

	"github.com/gorilla/websocket"
)

func TestOnlineUserStoreRegisterAndUnregister(t *testing.T) {
	store := NewOnlineUserStore()
	connA := &ChatConn{UserID: 1}
	connB := &ChatConn{UserID: 1}
	connC := &ChatConn{UserID: 2}

	store.Register(connA)
	store.Register(connB)
	store.Register(connC)

	if !store.IsOnline(1) {
		t.Fatal("用户 1 应在线")
	}
	if store.Count(1) != 2 {
		t.Fatalf("用户 1 在线连接数错误: %d", store.Count(1))
	}
	if store.Count(2) != 1 {
		t.Fatalf("用户 2 在线连接数错误: %d", store.Count(2))
	}

	snapshot := store.Snapshot(1)
	if len(snapshot) != 2 {
		t.Fatalf("用户 1 快照数量错误: %d", len(snapshot))
	}

	store.Unregister(connA)
	if store.Count(1) != 1 {
		t.Fatalf("用户 1 移除一条连接后数量错误: %d", store.Count(1))
	}

	store.Unregister(connB)
	if store.IsOnline(1) {
		t.Fatal("用户 1 应离线")
	}
}

func TestOnlineUserStorePushToUser(t *testing.T) {
	store := NewOnlineUserStore()
	serverConnA, clientConnA := mustNewWebSocketPair(t)
	defer clientConnA.Close()
	serverConnB, clientConnB := mustNewWebSocketPair(t)
	defer clientConnB.Close()

	goodConn := NewChatConn(1, serverConnA)
	badConn := NewChatConn(1, serverConnB)
	store.Register(goodConn)
	store.Register(badConn)

	if err := badConn.Close(); err != nil {
		t.Fatalf("关闭坏连接失败: %v", err)
	}

	successCount := store.PushToUser(1, websocket.TextMessage, []byte("hello"))
	if successCount != 1 {
		t.Fatalf("推送结果错误 success=%d", successCount)
	}
	if store.Count(1) != 1 {
		t.Fatalf("坏连接应被移除, 当前数量=%d", store.Count(1))
	}

	_, data, err := clientConnA.ReadMessage()
	if err != nil {
		t.Fatalf("读取推送消息失败: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("推送消息内容错误: %s", string(data))
	}
}

func TestOnlineUserStoreSnapshotIsIndependent(t *testing.T) {
	store := NewOnlineUserStore()
	connA := &ChatConn{UserID: 1}
	connB := &ChatConn{UserID: 1}
	store.Register(connA)
	store.Register(connB)

	snapshot := store.Snapshot(1)
	store.Unregister(connA)

	if len(snapshot) != 2 {
		t.Fatalf("快照应保持原始长度: %d", len(snapshot))
	}
	if store.Count(1) != 1 {
		t.Fatalf("当前在线连接数错误: %d", store.Count(1))
	}
}

func mustNewWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	serverConnCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("升级 ws 失败: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):]
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("拨号 ws 失败: %v", err)
	}

	select {
	case serverConn := <-serverConnCh:
		t.Cleanup(func() {
			_ = serverConn.Close()
		})
		return serverConn, clientConn
	case <-time.After(3 * time.Second):
		t.Fatal("等待服务端 ws 连接超时")
		return nil, nil
	}
}

func setupChatServiceTestDB(t *testing.T) (*models.UserModel, *models.UserModel) {
	t.Helper()

	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ChatSessionModel{},
		&models.ChatMsgModel{},
	)

	userA := &models.UserModel{Username: "u1", Nickname: "u1"}
	userB := &models.UserModel{Username: "u2", Nickname: "u2"}
	if err := db.Create(userA).Error; err != nil {
		t.Fatalf("创建用户 A 失败: %v", err)
	}
	if err := db.Create(userB).Error; err != nil {
		t.Fatalf("创建用户 B 失败: %v", err)
	}
	return userA, userB
}

func mustGetSession(t *testing.T, userID, receiverID ctype.ID) models.ChatSessionModel {
	t.Helper()

	var session models.ChatSessionModel
	if err := testutil.DB().Take(&session, "user_id = ? and receiver_id = ?", userID, receiverID).Error; err != nil {
		t.Fatalf("查询会话失败 user=%d receiver=%d: %v", userID, receiverID, err)
	}
	return session
}

func TestValidateChatBase(t *testing.T) {
	t.Run("默认值补齐", func(t *testing.T) {
		req := ToChatRequest{
			SenderID:   1,
			ReceiverID: 2,
			MsgType:    chat_msg_enum.MsgTypeText,
			Content:    "hello",
		}

		if err := validateChatBase(&req); err != nil {
			t.Fatalf("validateChatBase 返回错误: %v", err)
		}
		if req.SendTime.IsZero() {
			t.Fatal("应补齐默认发送时间")
		}
		if req.MsgStatus != chat_msg_enum.MsgStatusSend {
			t.Fatalf("默认消息状态错误: %v", req.MsgStatus)
		}
	})

	t.Run("禁止给自己发私信", func(t *testing.T) {
		req := ToChatRequest{
			SenderID:   1,
			ReceiverID: 1,
			MsgType:    chat_msg_enum.MsgTypeText,
			Content:    "hello",
			SendTime:   time.Now(),
		}

		err := validateChatBase(&req)
		if err != nil {
			t.Fatalf("自发消息应允许, got=%v", err)
		}
	})
}

func TestToTextChatCreatesMessageAndSessions(t *testing.T) {
	userA, userB := setupChatServiceTestDB(t)
	sendTime := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)

	msg, err := ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userA.ID,
		ReceiverID: userB.ID,
		Text:       "hello",
		SendTime:   sendTime,
	})
	if err != nil {
		t.Fatalf("发送文本消息失败: %v", err)
	}

	if msg.MsgType != chat_msg_enum.MsgTypeText {
		t.Fatalf("消息类型错误: %v", msg.MsgType)
	}
	if msg.MsgStatus != chat_msg_enum.MsgStatusSend {
		t.Fatalf("消息状态错误: %v", msg.MsgStatus)
	}
	if msg.SessionID != buildSessionID(userA.ID, userB.ID) {
		t.Fatalf("session_id 错误: %s", msg.SessionID)
	}
	if !msg.SendTime.Equal(sendTime) {
		t.Fatalf("发送时间错误: %v", msg.SendTime)
	}

	var msgCount int64
	if err = testutil.DB().Model(&models.ChatMsgModel{}).Count(&msgCount).Error; err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("消息数量错误: %d", msgCount)
	}

	var sessionCount int64
	if err = testutil.DB().Model(&models.ChatSessionModel{}).Count(&sessionCount).Error; err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	if sessionCount != 2 {
		t.Fatalf("会话数量错误: %d", sessionCount)
	}

	senderSession := mustGetSession(t, userA.ID, userB.ID)
	receiverSession := mustGetSession(t, userB.ID, userA.ID)

	if senderSession.SessionID != msg.SessionID || receiverSession.SessionID != msg.SessionID {
		t.Fatal("双方会话应共享相同 session_id")
	}
	if senderSession.UnreadCount != 0 {
		t.Fatalf("发送方未读数错误: %d", senderSession.UnreadCount)
	}
	if receiverSession.UnreadCount != 1 {
		t.Fatalf("接收方未读数错误: %d", receiverSession.UnreadCount)
	}
	if senderSession.LastMsgContent != "hello" {
		t.Fatalf("发送方最后消息摘要错误: %s", senderSession.LastMsgContent)
	}
	if receiverSession.LastMsgContent != "hello" {
		t.Fatalf("接收方最后消息摘要错误: %s", receiverSession.LastMsgContent)
	}
}

func TestToTextChatReusesSessions(t *testing.T) {
	userA, userB := setupChatServiceTestDB(t)

	_, err := ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userA.ID,
		ReceiverID: userB.ID,
		Text:       "first",
		SendTime:   time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("第一次发送失败: %v", err)
	}

	secondMsg, err := ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userA.ID,
		ReceiverID: userB.ID,
		Text:       "second",
		SendTime:   time.Date(2026, 3, 12, 11, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("第二次发送失败: %v", err)
	}

	var sessionCount int64
	if err = testutil.DB().Model(&models.ChatSessionModel{}).Count(&sessionCount).Error; err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	if sessionCount != 2 {
		t.Fatalf("重复发送后会话数量错误: %d", sessionCount)
	}

	senderSession := mustGetSession(t, userA.ID, userB.ID)
	receiverSession := mustGetSession(t, userB.ID, userA.ID)

	if senderSession.LastMsgID != secondMsg.ID || receiverSession.LastMsgID != secondMsg.ID {
		t.Fatalf("最后消息 ID 未更新: sender=%d receiver=%d msg=%d", senderSession.LastMsgID, receiverSession.LastMsgID, secondMsg.ID)
	}
	if receiverSession.UnreadCount != 2 {
		t.Fatalf("接收方未读数应累加到 2, got=%d", receiverSession.UnreadCount)
	}
	if senderSession.UnreadCount != 0 {
		t.Fatalf("发送方未读数应保持 0, got=%d", senderSession.UnreadCount)
	}
}

func TestToTextChatRestoresDeletedSession(t *testing.T) {
	userA, userB := setupChatServiceTestDB(t)

	_, err := ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userA.ID,
		ReceiverID: userB.ID,
		Text:       "first",
		SendTime:   time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("第一次发送失败: %v", err)
	}

	var deletedSession models.ChatSessionModel
	if err := testutil.DB().Take(&deletedSession, "user_id = ? and receiver_id = ?", userA.ID, userB.ID).Error; err != nil {
		t.Fatalf("查询待删除会话失败: %v", err)
	}
	if err := testutil.DB().Delete(&deletedSession).Error; err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}

	_, err = ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userB.ID,
		ReceiverID: userA.ID,
		Text:       "second",
		SendTime:   time.Date(2026, 3, 12, 11, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("第二次发送失败: %v", err)
	}

	var restoredSession models.ChatSessionModel
	if err := testutil.DB().Take(&restoredSession, "user_id = ? and receiver_id = ?", userA.ID, userB.ID).Error; err != nil {
		t.Fatalf("被删除的会话应被恢复: %v", err)
	}
	if restoredSession.DeletedAt.Valid {
		t.Fatalf("会话恢复后不应仍为软删状态: %+v", restoredSession)
	}
	if restoredSession.UnreadCount != 1 {
		t.Fatalf("恢复后的会话未读数应为 1, got=%d", restoredSession.UnreadCount)
	}
}

func TestToTextChatSupportsSelfChat(t *testing.T) {
	userA, _ := setupChatServiceTestDB(t)
	sendTime := time.Date(2026, 3, 13, 18, 10, 0, 0, time.Local)

	msg, err := ToTextChat(testutil.DB(), testutil.Logger(), ToTextChatRequest{
		SenderID:   userA.ID,
		ReceiverID: userA.ID,
		Text:       "备忘一下",
		SendTime:   sendTime,
	})
	if err != nil {
		t.Fatalf("自发消息失败: %v", err)
	}

	if msg.SessionID != buildSessionID(userA.ID, userA.ID) {
		t.Fatalf("自发消息 session_id 错误: %s", msg.SessionID)
	}

	var sessionCount int64
	if err = testutil.DB().Model(&models.ChatSessionModel{}).Count(&sessionCount).Error; err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("自发消息只应创建 1 条会话, got=%d", sessionCount)
	}

	session := mustGetSession(t, userA.ID, userA.ID)
	if session.LastMsgID != msg.ID {
		t.Fatalf("最后消息 ID 错误: %d", session.LastMsgID)
	}
	if session.LastMsgContent != "备忘一下" {
		t.Fatalf("最后消息摘要错误: %s", session.LastMsgContent)
	}
	if session.UnreadCount != 0 {
		t.Fatalf("自发消息未读数应保持 0, got=%d", session.UnreadCount)
	}
}

func TestToImageChatStoresJSONAndUpdatesSession(t *testing.T) {
	userA, userB := setupChatServiceTestDB(t)

	msg, err := ToImageChat(testutil.DB(), testutil.Logger(), ToImageChatRequest{
		SenderID:    userA.ID,
		ReceiverID:  userB.ID,
		ImageURL:    "https://cdn.example.com/image.png",
		PreviewURL:  "https://cdn.example.com/image_small.png",
		FileName:    "image.png",
		MimeType:    "image/png",
		Width:       100,
		Height:      200,
		Size:        4096,
		Alt:         "demo",
		OriginalURL: "https://cdn.example.com/image_origin.png",
		SendTime:    time.Date(2026, 3, 12, 12, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("发送图片消息失败: %v", err)
	}

	if msg.MsgType != chat_msg_enum.MsgTypeImage {
		t.Fatalf("消息类型错误: %v", msg.MsgType)
	}

	var payload imageChatContent
	if err = json.Unmarshal([]byte(msg.Content), &payload); err != nil {
		t.Fatalf("图片消息内容不是合法 JSON: %v", err)
	}
	if payload.ImageURL != "https://cdn.example.com/image.png" {
		t.Fatalf("图片地址错误: %s", payload.ImageURL)
	}
	if payload.PreviewURL != "https://cdn.example.com/image_small.png" {
		t.Fatalf("缩略图地址错误: %s", payload.PreviewURL)
	}

	senderSession := mustGetSession(t, userA.ID, userB.ID)
	receiverSession := mustGetSession(t, userB.ID, userA.ID)

	if senderSession.LastMsgContent != "[图片]" || receiverSession.LastMsgContent != "[图片]" {
		t.Fatalf("图片消息摘要错误: sender=%s receiver=%s", senderSession.LastMsgContent, receiverSession.LastMsgContent)
	}
	if receiverSession.UnreadCount != 1 {
		t.Fatalf("图片消息应给接收方增加 1 条未读, got=%d", receiverSession.UnreadCount)
	}
}
