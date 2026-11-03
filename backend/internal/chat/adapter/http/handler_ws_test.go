package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"myblogx/internal/apperr"
	chathttp "myblogx/internal/chat/adapter/http"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
)

// 这一组用例从 WS 这条真实入口观察"发送的结局"（#202）：
//   - 自己发给自己：成功时必须**回执**（现状是成功反而无回执）；
//   - 陌生人被站点开关拒绝：回执里必须是那条明确的站点策略原因。
//
// 为什么必须走 WebSocket 而不是只测用例层：回执是适配器行为，
// 只测 Service.Send 看不到帧，也就钉不住"成功却什么都不回"这个 bug。

type fakeRepo struct {
	sent int
}

func (f *fakeRepo) SendMessage(_ context.Context, msg *domain.Message, _, _ domain.Display, _ string) error {
	f.sent++
	msg.ID = ctype.NewID(uint64(f.sent))
	return nil
}

func (f *fakeRepo) ListSessions(context.Context, ctype.ID, bool, domain.Page) ([]domain.Session, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) FindSession(context.Context, string, ctype.ID, bool) (domain.Session, error) {
	return domain.Session{}, nil
}

func (f *fakeRepo) ListMessages(context.Context, domain.MessageQuery) ([]domain.Message, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) UserDeletedAt(context.Context, ctype.ID, string, []ctype.ID) (map[ctype.ID]time.Time, error) {
	return nil, nil
}

func (f *fakeRepo) MarkRead(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) DeleteMessagesForUser(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) DeleteSessions(context.Context, ctype.ID, []string) ([]string, error) {
	return nil, nil
}

type fakeUsers struct{}

func (fakeUsers) Displays(_ context.Context, ids []ctype.ID) (map[ctype.ID]domain.Display, error) {
	out := make(map[ctype.ID]domain.Display, len(ids))
	for _, id := range ids {
		out[id] = domain.Display{Nickname: "用户" + id.String()}
	}
	return out, nil
}

type fakeRelations struct{ relation int8 }

func (f fakeRelations) Batch(_ context.Context, _ ctype.ID, ids []ctype.ID) (map[ctype.ID]int8, error) {
	out := make(map[ctype.ID]int8, len(ids))
	for _, id := range ids {
		out[id] = f.relation
	}
	return out, nil
}

type fakeStranger struct{ enabled bool }

func (f fakeStranger) StrangerEnabled(context.Context) (bool, error) { return f.enabled, nil }

type fakeTickets struct {
	userID    uint64
	sessionID uint64
}

func (fakeTickets) Store(context.Context, string, uint64, uint64, time.Duration) error { return nil }

func (f fakeTickets) Consume(context.Context, string) (uint64, uint64, bool, error) {
	return f.userID, f.sessionID, true, nil
}

type fakeAuth struct{}

func (fakeAuth) Authenticate(context.Context, string) (uint64, uint64, bool, bool) {
	return 0, 0, false, false
}

func (fakeAuth) SessionActive(context.Context, uint64, uint64) (bool, error) { return true, nil }

// dial 起一个真实 HTTP 服务并连上 /chat/ws，返回连接。
func dialChat(t *testing.T, svc *app.Service) *websocket.Conn {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := chathttp.NewHandler(svc, ws.NewHub(), fakeTickets{userID: 1, sessionID: 1}, fakeAuth{}, nil)
	engine.GET("/chat/ws", handler.Ws)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/chat/ws?ticket=t"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接 WS 失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// readFrame 读一帧并按字段名判断它是什么。
//
// 服务端在成功路径上会先推一份消息实体给接收方（自己发给自己时就是推给
// 发送者本人），再发响应壳；两者都是 JSON，不能靠"读到的第一帧"断言。
func readFrame(t *testing.T, conn *websocket.Conn) (string, []byte) {
	t.Helper()
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取 WS 帧失败（可能根本没发回执）: %v", err)
	}
	return string(data), data
}

// readReceipt 跳过推送帧，等到带 code/msg 的响应壳。
func readReceipt(t *testing.T, conn *websocket.Conn) res.Response {
	t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		raw, data := readFrame(t, conn)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("帧不是 JSON 对象: %v，原文 %s", err, raw)
		}
		// 推送帧是消息实体（有 id / sender_id / content），没有 code。
		if _, isReceipt := fields["code"]; !isReceipt {
			continue
		}
		var envelope res.Response
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("回执不是响应壳: %v，原文 %s", err, raw)
		}
		return envelope
	}
	t.Fatal("没有等到发送回执（成功时也必须回执）")
	return res.Response{}
}

func sendRequest(t *testing.T, conn *websocket.Conn, receiverID uint64, content string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"receiver_id": ctype.NewID(receiverID),
		"msg_type":    1,
		"content":     content,
	})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("发送 WS 请求失败: %v", err)
	}
}

func TestWs_自己发给自己成功时也回执(t *testing.T) {
	svc := app.New(&fakeRepo{}, nil, fakeUsers{}, fakeStranger{enabled: false}, nil, nil, nil)
	conn := dialChat(t, svc)

	sendRequest(t, conn, 1, "自言自语")

	got := readReceipt(t, conn)
	if got.Code != res.SuccessCode {
		t.Fatalf("自己发给自己成功时应回成功回执，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if got.Msg != "给自己发送消息" {
		t.Fatalf("回执文案应为「给自己发送消息」，实际 %q", got.Msg)
	}
}

func TestWs_陌生人被站点开关拒绝时回执说明站点策略(t *testing.T) {
	svc := app.New(
		&fakeRepo{},
		fakeRelations{relation: 1}, // 陌生人
		fakeUsers{},
		fakeStranger{enabled: false},
		nil, nil, nil,
	)
	conn := dialChat(t, svc)

	sendRequest(t, conn, 2, "你好")

	got := readReceipt(t, conn)
	if got.Code != res.FailValidCode {
		t.Fatalf("被拒绝时应回失败回执，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	// 读者侧显示的就是这一串文案。必须是既有错误码对应的那条明确原因，
	// 而不是随便一句"发送失败"。
	if got.Msg != apperr.Message(apperr.CodeChatStrangerOff) {
		t.Fatalf("回执文案应为 %q，实际 %q", apperr.Message(apperr.CodeChatStrangerOff), got.Msg)
	}
	if !strings.Contains(got.Msg, "站点") || !strings.Contains(got.Msg, "陌生人") {
		t.Fatalf("拒绝原因必须点明是站点陌生人私信开关，实际 %q", got.Msg)
	}
}
