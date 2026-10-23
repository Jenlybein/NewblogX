package chat_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/models/enum/chat_msg_enum"
	"myblogx/models/enum/relationship_enum"
	"myblogx/service/chat_service"
	"myblogx/service/redis_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type chatListTestResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

type chatListPayload struct {
	List  []ChatSessionListResponse `json:"list"`
	Count int                       `json:"count"`
}

type chatMsgListPayload struct {
	List  []ChatMsgResponse `json:"list"`
	Count int               `json:"count"`
}

func newChatAPI() ChatApi {
	return New(Deps{
		DB:     testutil.DB(),
		Logger: testutil.Logger(),
		JWT:    testutil.Config().Jwt,
		Redis:  testutil.Redis(),
	})
}

// 会话删除与会话列表测试。
func TestChatSessionDeleteUserView(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	msgs := []models.ChatMsgModel{
		{SessionID: "chat:1:2", SenderID: users.owner.ID, ReceiverID: users.friendA.ID, Content: "a"},
		{SessionID: "chat:1:2", SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "b"},
		{SessionID: "chat:1:3", SenderID: users.owner.ID, ReceiverID: users.friendB.ID, Content: "c"},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息数据失败: %v", err)
	}

	rows := []models.ChatSessionModel{
		{SessionID: "chat:1:2", UserID: users.owner.ID, ReceiverID: users.friendA.ID},
		{SessionID: "chat:1:2", UserID: users.friendA.ID, ReceiverID: users.owner.ID},
		{SessionID: "chat:1:3", UserID: users.owner.ID, ReceiverID: users.friendB.ID},
		{SessionID: "chat:4:2", UserID: users.other.ID, ReceiverID: users.friendA.ID},
	}
	if err := testutil.DB().Create(&rows).Error; err != nil {
		t.Fatalf("创建会话数据失败: %v", err)
	}

	c, w := newChatDeleteCtx(t, users.owner, ChatSessionDeleteUserRequest{
		SessionIDList: []string{"chat:1:2", "chat:4:2", "not-exist"},
	})
	api.ChatSessionDeleteUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("删除会话应成功, body=%s", w.Body.String())
	}

	var ownerVisibleCount int64
	if err := testutil.DB().Model(&models.ChatSessionModel{}).
		Where("user_id = ?", users.owner.ID).
		Count(&ownerVisibleCount).Error; err != nil {
		t.Fatalf("统计用户可见会话失败: %v", err)
	}
	if ownerVisibleCount != 1 {
		t.Fatalf("当前用户剩余可见会话数量错误: %d", ownerVisibleCount)
	}

	var ownerDeleted models.ChatSessionModel
	if err := testutil.DB().Unscoped().
		Take(&ownerDeleted, "user_id = ? and session_id = ?", users.owner.ID, "chat:1:2").Error; err != nil {
		t.Fatalf("查询被删会话失败: %v", err)
	}
	if !ownerDeleted.DeletedAt.Valid {
		t.Fatalf("当前用户会话应被软删: %+v", ownerDeleted)
	}
	if ownerDeleted.ClearBeforeMsgID != msgs[1].ID {
		t.Fatalf("删除会话后应记录清空水位, got=%d", ownerDeleted.ClearBeforeMsgID)
	}

	var peerSession models.ChatSessionModel
	if err := testutil.DB().Take(&peerSession, "user_id = ? and session_id = ?", users.friendA.ID, "chat:1:2").Error; err != nil {
		t.Fatalf("对端会话不应被删除: %v", err)
	}

	var otherUserSession models.ChatSessionModel
	if err := testutil.DB().Take(&otherUserSession, "user_id = ? and session_id = ?", users.other.ID, "chat:4:2").Error; err != nil {
		t.Fatalf("他人会话不应被删除: %v", err)
	}

	var stateCount int64
	if err := testutil.DB().Unscoped().Model(&models.ChatMsgUserStateModel{}).
		Where("user_id = ? and session_id = ?", users.owner.ID, "chat:1:2").
		Count(&stateCount).Error; err != nil {
		t.Fatalf("统计消息用户态失败: %v", err)
	}
	if stateCount != 0 {
		t.Fatalf("删除会话后不应批量写消息用户态, got=%d", stateCount)
	}
}

func TestChatSessionDeleteUserViewRequiresList(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	c, w := newChatDeleteCtx(t, users.owner, ChatSessionDeleteUserRequest{})
	api.ChatSessionDeleteUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code == 0 {
		t.Fatalf("空列表应失败, body=%s", w.Body.String())
	}
}

func TestChatSessionListView(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)
	rows := []models.ChatSessionModel{
		{
			SessionID:      "chat:1:2",
			UserID:         users.owner.ID,
			ReceiverID:     users.friendA.ID,
			LastMsgID:      101,
			LastMsgContent: "first",
			LastMsgTime:    timePtr(now.Add(-2 * time.Hour)),
			UnreadCount:    3,
		},
		{
			SessionID:      "chat:1:3",
			UserID:         users.owner.ID,
			ReceiverID:     users.friendB.ID,
			LastMsgID:      102,
			LastMsgContent: "top",
			LastMsgTime:    timePtr(now.Add(-3 * time.Hour)),
			IsTop:          true,
		},
		{
			SessionID:      "chat:4:5",
			UserID:         users.other.ID,
			ReceiverID:     users.friendA.ID,
			LastMsgID:      103,
			LastMsgContent: "other",
			LastMsgTime:    timePtr(now),
		},
	}
	if err := testutil.DB().Create(&rows).Error; err != nil {
		t.Fatalf("创建会话数据失败: %v", err)
	}

	c, w := newChatListCtx(t, users.owner, ChatSessionListRequest{
		PageInfo: common.PageInfo{Page: 1, Limit: 10},
		Type:     1,
	})
	api.ChatSessionListView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("chat_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 2 {
		t.Fatalf("会话数量错误: %d", resp.Data.Count)
	}
	if len(resp.Data.List) != 2 {
		t.Fatalf("返回列表长度错误: %d", len(resp.Data.List))
	}

	if resp.Data.List[0].ReceiverID != users.friendB.ID {
		t.Fatalf("置顶会话应排第一: %+v", resp.Data.List[0])
	}
	if resp.Data.List[0].ReceiverNickname != users.friendB.Nickname {
		t.Fatalf("应带出对端昵称: %+v", resp.Data.List[0])
	}
	if int(resp.Data.List[0].Relation) != int(relationship_enum.RelationFans) {
		t.Fatalf("friendB 关系字段异常: %+v", resp.Data.List[0])
	}

	if resp.Data.List[1].ReceiverID != users.friendA.ID {
		t.Fatalf("非置顶会话顺序错误: %+v", resp.Data.List[1])
	}
	if resp.Data.List[1].UnreadCount != 3 {
		t.Fatalf("未读数错误: %+v", resp.Data.List[1])
	}
	if int(resp.Data.List[1].Relation) != int(relationship_enum.RelationFollowed) {
		t.Fatalf("friendA 关系字段异常: %+v", resp.Data.List[1])
	}
	expectTime := time.Date(2026, 3, 12, 7, 0, 0, 0, time.Local)
	if resp.Data.List[1].LastMsgTime == nil || !resp.Data.List[1].LastMsgTime.Equal(expectTime) {
		t.Fatalf("时间错误: %v", resp.Data.List[1].LastMsgTime)
	}
	if resp.Data.List[0].DeletedAt != nil || resp.Data.List[1].DeletedAt != nil {
		t.Fatalf("未删除会话不应返回 deleted_at: %+v", resp.Data.List)
	}
}

// 消息列表、消息删除、消息已读测试。
func TestChatMsgListView(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"
	sendTimeA := time.Date(2026, 3, 12, 8, 0, 0, 0, time.Local)
	sendTimeB := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)

	sessions := []models.ChatSessionModel{
		{SessionID: sessionID, UserID: users.owner.ID, ReceiverID: users.friendA.ID},
		{SessionID: sessionID, UserID: users.friendA.ID, ReceiverID: users.owner.ID},
	}
	if err := testutil.DB().Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	readAt := sendTimeB.Add(10 * time.Minute)
	msgs := []models.ChatMsgModel{
		{
			SessionID:  sessionID,
			SenderID:   users.owner.ID,
			ReceiverID: users.friendA.ID,
			Content:    "old",
			SendTime:   sendTimeA,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
		{
			SessionID:  sessionID,
			SenderID:   users.friendA.ID,
			ReceiverID: users.owner.ID,
			Content:    "new",
			SendTime:   sendTimeB,
			ReadAt:     &readAt,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusRead,
		},
		{
			SessionID:  "chat:3:4",
			SenderID:   users.other.ID,
			ReceiverID: users.friendB.ID,
			Content:    "other",
			SendTime:   sendTimeB,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	c, w := newChatMsgListCtx(t, users.owner, ChatMsgListRequest{
		PageInfo:  common.PageInfo{Page: 1, Limit: 10},
		SessionID: sessionID,
		Type:      1,
	})
	api.ChatMsgListView(c)

	resp := readChatMsgListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("chat_msg_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 2 || len(resp.Data.List) != 2 {
		t.Fatalf("消息列表数量错误: %+v", resp.Data)
	}
	if resp.Data.List[0].Content != "new" {
		t.Fatalf("应按时间倒序返回最新消息: %+v", resp.Data.List[0])
	}
	if resp.Data.List[0].IsSelf {
		t.Fatalf("对方消息不应标记为自己发送: %+v", resp.Data.List[0])
	}
	if !resp.Data.List[0].IsRead {
		t.Fatalf("已读消息应标记 IsRead=true: %+v", resp.Data.List[0])
	}
	if !resp.Data.List[1].IsSelf {
		t.Fatalf("自己发送的消息应标记 IsSelf=true: %+v", resp.Data.List[1])
	}
	if resp.Data.List[1].IsRead {
		t.Fatalf("未读消息不应标记 IsRead=true: %+v", resp.Data.List[1])
	}
	if resp.Data.List[0].DeletedAt != nil || resp.Data.List[1].DeletedAt != nil {
		t.Fatalf("未删除消息不应返回 deleted_at: %+v", resp.Data.List)
	}
}

func TestChatMsgListViewFiltersDeletedByUserState(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"
	sendTimeA := time.Date(2026, 3, 12, 8, 0, 0, 0, time.Local)
	sendTimeB := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)

	sessions := []models.ChatSessionModel{
		{SessionID: sessionID, UserID: users.owner.ID, ReceiverID: users.friendA.ID},
		{SessionID: sessionID, UserID: users.friendA.ID, ReceiverID: users.owner.ID},
	}
	if err := testutil.DB().Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	msgs := []models.ChatMsgModel{
		{
			SessionID:  sessionID,
			SenderID:   users.owner.ID,
			ReceiverID: users.friendA.ID,
			Content:    "old",
			SendTime:   sendTimeA,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
		{
			SessionID:  sessionID,
			SenderID:   users.friendA.ID,
			ReceiverID: users.owner.ID,
			Content:    "hidden",
			SendTime:   sendTimeB,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}
	if err := testutil.DB().Create(&models.ChatMsgUserStateModel{
		Model: models.Model{
			DeletedAt: gorm.DeletedAt{Time: sendTimeB.Add(time.Minute), Valid: true},
		},
		MsgID:     msgs[1].ID,
		UserID:    users.owner.ID,
		SessionID: sessionID,
	}).Error; err != nil {
		t.Fatalf("创建消息用户态失败: %v", err)
	}

	c, w := newChatMsgListCtx(t, users.owner, ChatMsgListRequest{
		PageInfo:  common.PageInfo{Page: 1, Limit: 10},
		SessionID: sessionID,
		Type:      1,
	})
	api.ChatMsgListView(c)

	resp := readChatMsgListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("chat_msg_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 1 || len(resp.Data.List) != 1 {
		t.Fatalf("已删除消息应被过滤: %+v", resp.Data)
	}
	if resp.Data.List[0].Content != "old" {
		t.Fatalf("剩余消息错误: %+v", resp.Data.List[0])
	}
}

func TestChatMsgDeleteUserView(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"

	msgs := []models.ChatMsgModel{
		{SessionID: sessionID, SenderID: users.owner.ID, ReceiverID: users.friendA.ID, Content: "a"},
		{SessionID: sessionID, SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "b"},
		{SessionID: "chat:4:5", SenderID: users.other.ID, ReceiverID: users.friendB.ID, Content: "c"},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	c, w := newChatMsgDeleteCtx(t, users.owner, ChatMsgDeleteUserRequest{
		MsgIDList: []ctype.ID{msgs[0].ID, msgs[1].ID, msgs[2].ID, 99999},
	})
	api.ChatMsgDeleteUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("删除消息应成功, body=%s", w.Body.String())
	}

	var stateList []models.ChatMsgUserStateModel
	if err := testutil.DB().Unscoped().Find(&stateList, "user_id = ? and deleted_at is not null", users.owner.ID).Error; err != nil {
		t.Fatalf("查询消息用户态失败: %v", err)
	}
	if len(stateList) != 2 {
		t.Fatalf("应只删除当前用户可见的 2 条消息, got=%d", len(stateList))
	}
}

func TestChatMsgReadUserView(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	sessions := []models.ChatSessionModel{
		{SessionID: "chat:1:2", UserID: users.owner.ID, ReceiverID: users.friendA.ID, UnreadCount: 2},
		{SessionID: "chat:1:3", UserID: users.owner.ID, ReceiverID: users.friendB.ID, UnreadCount: 1},
	}
	if err := testutil.DB().Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	msgs := []models.ChatMsgModel{
		{SessionID: "chat:1:2", SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "m1", MsgStatus: chat_msg_enum.MsgStatusSend},
		{SessionID: "chat:1:2", SenderID: users.owner.ID, ReceiverID: users.friendA.ID, Content: "self", MsgStatus: chat_msg_enum.MsgStatusSend},
		{SessionID: "chat:1:2", SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "m2", MsgStatus: chat_msg_enum.MsgStatusDelivered},
		{SessionID: "chat:1:3", SenderID: users.friendB.ID, ReceiverID: users.owner.ID, Content: "m3", MsgStatus: chat_msg_enum.MsgStatusSend},
		{SessionID: "chat:4:5", SenderID: users.other.ID, ReceiverID: users.friendB.ID, Content: "other", MsgStatus: chat_msg_enum.MsgStatusSend},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	c, w := newChatMsgReadCtx(t, users.owner, ChatMsgReadUserRequest{
		MsgIDList: []ctype.ID{msgs[0].ID, msgs[1].ID, msgs[3].ID, msgs[4].ID, 99999},
	})
	api.ChatMsgReadUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("批量已读应成功, body=%s", w.Body.String())
	}

	var readMsgs []models.ChatMsgModel
	if err := testutil.DB().Find(&readMsgs, "id IN ?", []ctype.ID{msgs[0].ID, msgs[3].ID}).Error; err != nil {
		t.Fatalf("查询已读消息失败: %v", err)
	}
	for _, item := range readMsgs {
		if item.MsgStatus != chat_msg_enum.MsgStatusRead || item.ReadAt == nil {
			t.Fatalf("目标消息应被标记已读: %+v", item)
		}
	}

	var selfMsg models.ChatMsgModel
	if err := testutil.DB().Take(&selfMsg, "id = ?", msgs[1].ID).Error; err != nil {
		t.Fatalf("查询自己发送的消息失败: %v", err)
	}
	if selfMsg.MsgStatus != chat_msg_enum.MsgStatusSend || selfMsg.ReadAt != nil {
		t.Fatalf("自己发送的消息不应被标记已读: %+v", selfMsg)
	}

	var sessionA models.ChatSessionModel
	if err := testutil.DB().Take(&sessionA, "user_id = ? and session_id = ?", users.owner.ID, "chat:1:2").Error; err != nil {
		t.Fatalf("查询会话A失败: %v", err)
	}
	if sessionA.UnreadCount != 1 {
		t.Fatalf("会话A未读数应重算为 1, got=%d", sessionA.UnreadCount)
	}

	var sessionB models.ChatSessionModel
	if err := testutil.DB().Take(&sessionB, "user_id = ? and session_id = ?", users.owner.ID, "chat:1:3").Error; err != nil {
		t.Fatalf("查询会话B失败: %v", err)
	}
	if sessionB.UnreadCount != 0 {
		t.Fatalf("会话B未读数应重算为 0, got=%d", sessionB.UnreadCount)
	}
}

func TestChatMsgReadUserViewPushesReadReceiptToSender(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	serverConn, clientConn := mustNewChatAPITestWebSocketPair(t)
	defer clientConn.Close()

	onlineConn := chat_service.NewChatConn(users.friendA.ID, serverConn)
	store := chat_service.GetOnlineUserStore()
	store.Register(onlineConn)
	defer store.Unregister(onlineConn)
	defer onlineConn.Close()

	session := models.ChatSessionModel{
		SessionID:      "chat:1:2",
		UserID:         users.owner.ID,
		ReceiverID:     users.friendA.ID,
		UnreadCount:    1,
		LastMsgTime:    timePtr(time.Now()),
		LastMsgID:      1,
		LastMsgContent: "m1",
	}
	if err := testutil.DB().Create(&session).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	msg := models.ChatMsgModel{
		SessionID:  "chat:1:2",
		SenderID:   users.friendA.ID,
		ReceiverID: users.owner.ID,
		Content:    "m1",
		MsgStatus:  chat_msg_enum.MsgStatusSend,
		SendTime:   time.Now(),
	}
	if err := testutil.DB().Create(&msg).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	c, w := newChatMsgReadCtx(t, users.owner, ChatMsgReadUserRequest{
		MsgIDList: []ctype.ID{msg.ID},
	})
	api.ChatMsgReadUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("批量已读应成功, body=%s", w.Body.String())
	}

	if err := clientConn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("设置读取超时失败: %v", err)
	}
	_, payload, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("发送方应收到已读回执: %v", err)
	}

	var wsResp chatListTestResponse
	if err := json.Unmarshal(payload, &wsResp); err != nil {
		t.Fatalf("解析 ws 响应失败: %v payload=%s", err, string(payload))
	}
	if wsResp.Code != 0 {
		t.Fatalf("ws 回执 code 错误: %+v", wsResp)
	}

	var push ChatMsgReadPush
	if err := json.Unmarshal(wsResp.Data, &push); err != nil {
		t.Fatalf("解析已读回执失败: %v body=%s", err, string(payload))
	}
	if push.MsgType != chat_msg_enum.MsgTypeRead {
		t.Fatalf("回执类型错误: %+v", push)
	}
	if push.ReaderID != users.owner.ID {
		t.Fatalf("ReaderID 错误: %+v", push)
	}
	if push.SessionID != "chat:1:2" {
		t.Fatalf("SessionID 错误: %+v", push)
	}
	if len(push.MsgIDList) != 1 || push.MsgIDList[0] != msg.ID {
		t.Fatalf("MsgIDList 错误: %+v", push)
	}
	if push.ReadAt.IsZero() {
		t.Fatalf("ReadAt 不应为空: %+v", push)
	}
}

func TestChatMsgReadUserViewDecreasesUnreadCountByMatchedMessages(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"

	msgs := []models.ChatMsgModel{
		{SessionID: sessionID, SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "cleared", MsgStatus: chat_msg_enum.MsgStatusSend},
		{SessionID: sessionID, SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "deleted", MsgStatus: chat_msg_enum.MsgStatusSend},
		{SessionID: sessionID, SenderID: users.friendA.ID, ReceiverID: users.owner.ID, Content: "visible", MsgStatus: chat_msg_enum.MsgStatusSend},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	session := models.ChatSessionModel{
		SessionID:        sessionID,
		UserID:           users.owner.ID,
		ReceiverID:       users.friendA.ID,
		ClearBeforeMsgID: msgs[0].ID,
		UnreadCount:      3,
	}
	if err := testutil.DB().Create(&session).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	if err := testutil.DB().Create(&models.ChatMsgUserStateModel{
		Model: models.Model{
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		},
		MsgID:     msgs[1].ID,
		UserID:    users.owner.ID,
		SessionID: sessionID,
	}).Error; err != nil {
		t.Fatalf("创建消息用户态失败: %v", err)
	}

	c, w := newChatMsgReadCtx(t, users.owner, ChatMsgReadUserRequest{
		MsgIDList: []ctype.ID{msgs[2].ID},
	})
	api.ChatMsgReadUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("批量已读应成功, body=%s", w.Body.String())
	}

	var updated models.ChatSessionModel
	if err := testutil.DB().Take(&updated, "user_id = ? and session_id = ?", users.owner.ID, sessionID).Error; err != nil {
		t.Fatalf("查询会话失败: %v", err)
	}
	if updated.UnreadCount != 2 {
		t.Fatalf("未读数应只按本次命中消息递减, got=%d", updated.UnreadCount)
	}
}

func TestChatMsgListViewFiltersByClearBeforeMsgID(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"
	sendTimeA := time.Date(2026, 3, 12, 8, 0, 0, 0, time.Local)
	sendTimeB := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)

	msgs := []models.ChatMsgModel{
		{
			SessionID:  sessionID,
			SenderID:   users.owner.ID,
			ReceiverID: users.friendA.ID,
			Content:    "old",
			SendTime:   sendTimeA,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
		{
			SessionID:  sessionID,
			SenderID:   users.friendA.ID,
			ReceiverID: users.owner.ID,
			Content:    "new",
			SendTime:   sendTimeB,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	sessions := []models.ChatSessionModel{
		{SessionID: sessionID, UserID: users.owner.ID, ReceiverID: users.friendA.ID, ClearBeforeMsgID: msgs[0].ID},
		{SessionID: sessionID, UserID: users.friendA.ID, ReceiverID: users.owner.ID},
	}
	if err := testutil.DB().Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	c, w := newChatMsgListCtx(t, users.owner, ChatMsgListRequest{
		PageInfo:  common.PageInfo{Page: 1, Limit: 10},
		SessionID: sessionID,
		Type:      1,
	})
	api.ChatMsgListView(c)

	resp := readChatMsgListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("chat_msg_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 1 || len(resp.Data.List) != 1 {
		t.Fatalf("清空水位前的消息应被过滤: %+v", resp.Data)
	}
	if resp.Data.List[0].Content != "new" {
		t.Fatalf("剩余消息错误: %+v", resp.Data.List[0])
	}
}

func TestChatMsgDeleteUserViewRequiresList(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	c, w := newChatMsgDeleteCtx(t, users.owner, ChatMsgDeleteUserRequest{})
	api.ChatMsgDeleteUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code == 0 {
		t.Fatalf("空消息列表应失败, body=%s", w.Body.String())
	}
}

func TestChatMsgReadUserViewRequiresList(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	c, w := newChatMsgReadCtx(t, users.owner, ChatMsgReadUserRequest{})
	api.ChatMsgReadUserView(c)

	resp := readChatListResponse(t, w)
	if resp.Code == 0 {
		t.Fatalf("空消息列表应失败, body=%s", w.Body.String())
	}
}

// 管理员视角的会话与消息列表测试。
func TestChatSessionListViewAdmin(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	rows := []models.ChatSessionModel{
		{
			SessionID:      "chat:1:2",
			UserID:         users.owner.ID,
			ReceiverID:     users.friendA.ID,
			LastMsgContent: "active",
			LastMsgTime:    timePtr(now),
		},
		{
			SessionID:      "chat:1:3",
			UserID:         users.owner.ID,
			ReceiverID:     users.friendB.ID,
			LastMsgContent: "deleted",
			LastMsgTime:    timePtr(now.Add(-time.Hour)),
		},
	}
	if err := testutil.DB().Create(&rows).Error; err != nil {
		t.Fatalf("创建会话数据失败: %v", err)
	}
	if err := testutil.DB().Where("session_id = ? and user_id = ?", "chat:1:3", users.owner.ID).
		Delete(&models.ChatSessionModel{}).Error; err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}

	c, w := newChatListCtxWithRole(t, users.owner, enum.RoleAdmin, ChatSessionListRequest{
		PageInfo: common.PageInfo{Page: 1, Limit: 10},
		UserID:   users.owner.ID,
		Type:     2,
	})
	api.ChatSessionListView(c)

	resp := readChatListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("管理员 chat_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 2 || len(resp.Data.List) != 2 {
		t.Fatalf("管理员会话数量错误: %+v", resp.Data)
	}
	if int(resp.Data.List[0].Relation) != int(relationship_enum.RelationFollowed) {
		t.Fatalf("管理员查看时第一条关系字段异常: %+v", resp.Data.List[0])
	}
	if int(resp.Data.List[1].Relation) != int(relationship_enum.RelationFans) {
		t.Fatalf("管理员查看时第二条关系字段异常: %+v", resp.Data.List[1])
	}
	if resp.Data.List[1].DeletedAt == nil || resp.Data.List[1].DeletedAt.IsZero() {
		t.Fatalf("管理员应看到软删时间: %+v", resp.Data.List[1])
	}
}

func TestChatSessionListViewAdminRequiresUserID(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	c, w := newChatListCtxWithRole(t, users.owner, enum.RoleAdmin, ChatSessionListRequest{
		PageInfo: common.PageInfo{Page: 1, Limit: 10},
		Type:     2,
	})
	api.ChatSessionListView(c)

	resp := readChatListResponse(t, w)
	if resp.Code == 0 {
		t.Fatalf("缺少 user_id 时应失败, body=%s", w.Body.String())
	}
}

func TestChatMsgListViewAdmin(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()
	sessionID := "chat:1:2"
	sendTimeA := time.Date(2026, 3, 12, 8, 0, 0, 0, time.Local)
	sendTimeB := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)

	sessions := []models.ChatSessionModel{
		{SessionID: sessionID, UserID: users.owner.ID, ReceiverID: users.friendA.ID},
		{SessionID: sessionID, UserID: users.friendA.ID, ReceiverID: users.owner.ID},
	}
	if err := testutil.DB().Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	if err := testutil.DB().Where("session_id = ? and user_id = ?", sessionID, users.owner.ID).
		Delete(&models.ChatSessionModel{}).Error; err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}

	msgs := []models.ChatMsgModel{
		{
			SessionID:  sessionID,
			SenderID:   users.owner.ID,
			ReceiverID: users.friendA.ID,
			Content:    "old",
			SendTime:   sendTimeA,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusSend,
		},
		{
			SessionID:  sessionID,
			SenderID:   users.friendA.ID,
			ReceiverID: users.owner.ID,
			Content:    "deleted",
			SendTime:   sendTimeB,
			MsgType:    chat_msg_enum.MsgTypeText,
			MsgStatus:  chat_msg_enum.MsgStatusRead,
		},
	}
	if err := testutil.DB().Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}
	if err := testutil.DB().Create(&models.ChatMsgUserStateModel{
		Model: models.Model{
			DeletedAt: gorm.DeletedAt{Time: sendTimeB.Add(time.Minute), Valid: true},
		},
		MsgID:     msgs[1].ID,
		UserID:    users.owner.ID,
		SessionID: sessionID,
	}).Error; err != nil {
		t.Fatalf("创建消息用户态失败: %v", err)
	}

	c, w := newChatMsgListCtxWithRole(t, users.owner, enum.RoleAdmin, ChatMsgListRequest{
		PageInfo:  common.PageInfo{Page: 1, Limit: 10},
		SessionID: sessionID,
		UserID:    users.owner.ID,
		Type:      2,
	})
	api.ChatMsgListView(c)

	resp := readChatMsgListResponse(t, w)
	if resp.Code != 0 {
		t.Fatalf("管理员 chat_msg_list 应成功, body=%s", w.Body.String())
	}
	if resp.Data.Count != 2 || len(resp.Data.List) != 2 {
		t.Fatalf("管理员消息数量错误: %+v", resp.Data)
	}
	if resp.Data.List[0].Content != "deleted" {
		t.Fatalf("管理员应看到软删消息: %+v", resp.Data.List[0])
	}
	if resp.Data.List[0].DeletedAt == nil || resp.Data.List[0].DeletedAt.IsZero() {
		t.Fatalf("管理员应看到消息软删时间: %+v", resp.Data.List[0])
	}
	if resp.Data.List[1].DeletedAt != nil {
		t.Fatalf("未删除消息不应返回 deleted_at: %+v", resp.Data.List[1])
	}
	if resp.Data.List[1].IsSelf != true {
		t.Fatalf("管理员查看时应按 user_id 计算 IsSelf: %+v", resp.Data.List[1])
	}
}

func TestChatMsgListViewAdminRequiresUserID(t *testing.T) {
	users := setupChatListEnv(t)
	api := newChatAPI()

	c, w := newChatMsgListCtxWithRole(t, users.owner, enum.RoleAdmin, ChatMsgListRequest{
		PageInfo:  common.PageInfo{Page: 1, Limit: 10},
		SessionID: "chat:1:2",
		Type:      2,
	})
	api.ChatMsgListView(c)

	resp := readChatMsgListResponse(t, w)
	if resp.Code == 0 {
		t.Fatalf("缺少 user_id 时应失败, body=%s", w.Body.String())
	}
}

type chatUsers struct {
	owner   models.UserModel
	friendA models.UserModel
	friendB models.UserModel
	other   models.UserModel
}

// 通用测试数据准备。
func setupChatListEnv(t *testing.T) chatUsers {
	t.Helper()
	testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserFollowModel{}, &models.ChatSessionModel{}, &models.ChatMsgModel{}, &models.ChatMsgUserStateModel{})

	users := chatUsers{
		owner:   createChatUser(t, "chat_owner"),
		friendA: createChatUser(t, "chat_friend_a"),
		friendB: createChatUser(t, "chat_friend_b"),
		other:   createChatUser(t, "chat_other"),
	}
	if err := testutil.DB().Create(&models.UserFollowModel{FollowedUserID: users.friendA.ID, FansUserID: users.owner.ID}).Error; err != nil {
		t.Fatalf("创建 owner->friendA 关注关系失败: %v", err)
	}
	if err := testutil.DB().Create(&models.UserFollowModel{FollowedUserID: users.owner.ID, FansUserID: users.friendB.ID}).Error; err != nil {
		t.Fatalf("创建 friendB->owner 关注关系失败: %v", err)
	}
	return users
}

func createChatUser(t *testing.T, username string) models.UserModel {
	t.Helper()
	user := models.UserModel{
		Username: username,
		Nickname: username + "_nick",
		Avatar:   username + ".png",
		Abstract: username + "_abstract",
	}
	if err := testutil.DB().Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return user
}

// 通用辅助函数。
func timePtr(t time.Time) *time.Time {
	return &t
}

func newChatListCtx(t *testing.T, user models.UserModel, query ChatSessionListRequest) (*gin.Context, *httptest.ResponseRecorder) {
	return newChatListCtxWithRole(t, user, enum.RoleUser, query)
}

func newChatListCtxWithRole(t *testing.T, user models.UserModel, role enum.RoleType, query ChatSessionListRequest) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/chat", nil)
	c.Set("requestQuery", query)
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     role,
			Username: user.Username,
		},
	})
	return c, w
}

func newChatMsgListCtx(t *testing.T, user models.UserModel, query ChatMsgListRequest) (*gin.Context, *httptest.ResponseRecorder) {
	return newChatMsgListCtxWithRole(t, user, enum.RoleUser, query)
}

func newChatDeleteCtx(t *testing.T, user models.UserModel, body ChatSessionDeleteUserRequest) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/chat/sessions", nil)
	c.Set("requestJson", body)
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     enum.RoleUser,
			Username: user.Username,
		},
	})
	return c, w
}

func newChatMsgDeleteCtx(t *testing.T, user models.UserModel, body ChatMsgDeleteUserRequest) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/chat/messages", nil)
	c.Set("requestJson", body)
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     enum.RoleUser,
			Username: user.Username,
		},
	})
	return c, w
}

func newChatMsgReadCtx(t *testing.T, user models.UserModel, body ChatMsgReadUserRequest) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/chat/messages/read", nil)
	c.Set("requestJson", body)
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     enum.RoleUser,
			Username: user.Username,
		},
	})
	return c, w
}

func newChatMsgListCtxWithRole(t *testing.T, user models.UserModel, role enum.RoleType, query ChatMsgListRequest) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/chat/msg", nil)
	c.Set("requestQuery", query)
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     role,
			Username: user.Username,
		},
	})
	return c, w
}

func readChatListResponse(t *testing.T, w *httptest.ResponseRecorder) struct {
	Code int
	Data chatListPayload
	Msg  string
} {
	t.Helper()

	var resp chatListTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}

	var payload chatListPayload
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			t.Fatalf("解析列表载荷失败: %v body=%s", err, w.Body.String())
		}
	}

	return struct {
		Code int
		Data chatListPayload
		Msg  string
	}{
		Code: resp.Code,
		Data: payload,
		Msg:  resp.Msg,
	}
}

func readChatMsgListResponse(t *testing.T, w *httptest.ResponseRecorder) struct {
	Code int
	Data chatMsgListPayload
	Msg  string
} {
	t.Helper()

	var resp chatListTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}

	var payload chatMsgListPayload
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			t.Fatalf("解析消息列表载荷失败: %v body=%s", err, w.Body.String())
		}
	}

	return struct {
		Code int
		Data chatMsgListPayload
		Msg  string
	}{
		Code: resp.Code,
		Data: payload,
		Msg:  resp.Msg,
	}
}

func mustNewChatAPITestWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
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

type chatWSPermissionUsers struct {
	sender   models.UserModel
	receiver models.UserModel
}

// ws 发送权限与限流测试。
func TestValidateChatSendPermissionStrangerDependsOnReceiverConfig(t *testing.T) {
	users := setupChatWSPermissionEnv(t)

	if err := testutil.DB().Model(&models.UserConfModel{}).
		Where("user_id = ?", users.receiver.ID).
		Update("stranger_chat_enabled", false).Error; err != nil {
		t.Fatalf("更新陌生人私信配置失败: %v", err)
	}
	if err := testutil.DB().Preload("UserConfModel").Take(&users.receiver, users.receiver.ID).Error; err != nil {
		t.Fatalf("查询接收人失败: %v", err)
	}

	reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
	if err == nil || err.Error() != "对方未开启陌生人私信" {
		t.Fatalf("陌生人应受接收配置限制, got=%v", err)
	}
	if reservation != nil {
		t.Fatal("受限时不应返回预占对象")
	}

	if err := testutil.DB().Model(&models.UserConfModel{}).
		Where("user_id = ?", users.receiver.ID).
		Update("stranger_chat_enabled", true).Error; err != nil {
		t.Fatalf("恢复陌生人私信配置失败: %v", err)
	}
	if err := testutil.DB().Preload("UserConfModel").Take(&users.receiver, users.receiver.ID).Error; err != nil {
		t.Fatalf("查询接收人失败: %v", err)
	}

	reservation, err = validateChatSendPermission(users.sender.ID, &users.receiver)
	if err != nil {
		t.Fatalf("开启陌生人私信后应允许发送: %v", err)
	}
	if reservation == nil {
		t.Fatal("允许发送时应返回预占对象")
	}
	if err := reservation.Rollback(); err != nil {
		t.Fatalf("回滚预占失败: %v", err)
	}
}

func TestValidateChatSendPermissionStrangerLimitedByWeeklyQuota(t *testing.T) {
	users := setupChatWSPermissionEnv(t)

	firstReservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
	if err != nil {
		t.Fatalf("陌生人首条消息应允许: %v", err)
	}
	if err := firstReservation.Commit(); err != nil {
		t.Fatalf("提交陌生人首条预占失败: %v", err)
	}

	secondReservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
	if err == nil || err.Error() != "本周只允许向陌生人发送 1 条消息" {
		t.Fatalf("陌生人每周第二条应受限, got=%v", err)
	}
	if secondReservation != nil {
		t.Fatal("陌生人周配额受限时不应返回预占对象")
	}
}

func TestValidateChatSendPermissionLimitedByWeeklyQuotaUntilReply(t *testing.T) {
	users := setupChatWSPermissionEnv(t)
	createFollowRelation(t, users.sender.ID, users.receiver.ID)

	for i := 0; i < 3; i++ {
		reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
		if err != nil {
			t.Fatalf("第 %d 次发送前置校验失败: %v", i+1, err)
		}
		if err := reservation.Commit(); err != nil {
			t.Fatalf("提交预占失败: %v", err)
		}
	}

	reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
	if err == nil || err.Error() != "本周可发送消息次数已达上限，对方回复后才可以继续发送消息" {
		t.Fatalf("单向关系超过自然周 3 条应受限, got=%v", err)
	}
	if reservation != nil {
		t.Fatal("受限时不应返回预占对象")
	}

	replyReservation, err := validateChatSendPermission(users.receiver.ID, &users.sender)
	if err != nil {
		t.Fatalf("对方回复前置校验失败: %v", err)
	}
	if err := replyReservation.Commit(); err != nil {
		t.Fatalf("提交回复预占失败: %v", err)
	}

	reservation, err = validateChatSendPermission(users.sender.ID, &users.receiver)
	if err != nil {
		t.Fatalf("对方回复后应重新允许发送: %v", err)
	}
	if reservation == nil {
		t.Fatal("对方回复后应返回预占对象")
	}
	if err := reservation.Rollback(); err != nil {
		t.Fatalf("回滚预占失败: %v", err)
	}
}

func TestValidateChatSendPermissionFriendAlwaysAllowed(t *testing.T) {
	users := setupChatWSPermissionEnv(t)
	createFollowRelation(t, users.sender.ID, users.receiver.ID)
	createFollowRelation(t, users.receiver.ID, users.sender.ID)

	for i := 0; i < 5; i++ {
		reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
		if err != nil {
			t.Fatalf("好友之间第 %d 次发送应允许: %v", i+1, err)
		}
		if err := reservation.Commit(); err != nil {
			t.Fatalf("提交预占失败: %v", err)
		}
	}
}

func TestValidateChatSendPermissionSessionMinuteLimit(t *testing.T) {
	users := setupChatWSPermissionEnv(t)
	createFollowRelation(t, users.sender.ID, users.receiver.ID)
	createFollowRelation(t, users.receiver.ID, users.sender.ID)

	for i := 0; i < 30; i++ {
		reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
		if err != nil {
			t.Fatalf("第 %d 次会话发送应允许: %v", i+1, err)
		}
		if err := reservation.Commit(); err != nil {
			t.Fatalf("提交预占失败: %v", err)
		}
	}

	reservation, err := validateChatSendPermission(users.sender.ID, &users.receiver)
	if err == nil || err.Error() != "当前会话发送过于频繁，请稍后再试" {
		t.Fatalf("同一会话 60 秒内第 31 条应受限, got=%v", err)
	}
	if reservation != nil {
		t.Fatal("受限时不应返回预占对象")
	}
}

// ws 权限测试专用数据准备。
func setupChatWSPermissionEnv(t *testing.T) chatWSPermissionUsers {
	t.Helper()

	_ = testutil.SetupMiniRedis(t)
	testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.UserFollowModel{},
	)

	sender := createChatWSPermissionUser(t, "ws_sender")
	receiver := createChatWSPermissionUser(t, "ws_receiver")
	return chatWSPermissionUsers{
		sender:   sender,
		receiver: receiver,
	}
}

func createChatWSPermissionUser(t *testing.T, username string) models.UserModel {
	t.Helper()

	user := models.UserModel{
		Username: username,
		Nickname: username + "_nick",
	}
	if err := testutil.DB().Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := testutil.DB().Preload("UserConfModel").Take(&user, user.ID).Error; err != nil {
		t.Fatalf("查询用户配置失败: %v", err)
	}
	return user
}

func createFollowRelation(t *testing.T, fansUserID, followedUserID ctype.ID) {
	t.Helper()

	row := models.UserFollowModel{
		FansUserID:     fansUserID,
		FollowedUserID: followedUserID,
	}
	if err := testutil.DB().Create(&row).Error; err != nil {
		t.Fatalf("创建关注关系失败: %v", err)
	}
}

func validateChatSendPermission(senderID ctype.ID, receiver *models.UserModel) (*chat_service.ChatSendReservation, error) {
	return chat_service.CheckAndReserveChatSend(testutil.DB(), redis_service.Deps{
		Client: testutil.Redis(),
		Logger: testutil.Logger(),
	}, senderID, receiver)
}
