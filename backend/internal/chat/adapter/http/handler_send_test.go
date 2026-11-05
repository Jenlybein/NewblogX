package http_test

import (
	"context"
	"encoding/json"
	"net/http"
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
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
	"myblogx/utils/jwts"
)

// 这一组用例从**适配器**观察发送的结局（#202 的原意，通道随 #201 变更）：
//   - 自己发给自己：成功时必须回执（旧实现成功反而无回执）；
//   - 陌生人被站点开关拒绝：回执里必须是那条明确的站点策略原因。
//
// #201 把发送从 WS 换到了 HTTP（POST /chat/messages），所以"发送的结局"现在
// 是 HTTP 响应体；WS 只剩通知通道，上行帧统一的结局是"请改用 HTTP"。
// 断言口径保持不变：成功看 code=0 的完整回执，被拒看既有错误码对应文案。

type fakeRepo struct {
	sent []*domain.Message
	err  error
}

func (f *fakeRepo) Send(_ context.Context, cmd domain.SendCommand, _, _ domain.Display, _ string) (domain.Message, bool, error) {
	if f.err != nil {
		return domain.Message{}, false, f.err
	}
	msg := domain.Message{
		ID:         ctype.NewID(uint64(len(f.sent) + 1)),
		SenderID:   cmd.SenderID,
		ReceiverID: cmd.ReceiverID,
		SessionID:  domain.BuildSessionID(cmd.SenderID, cmd.ReceiverID),
		Content:    cmd.Content,
		SendTime:   cmd.SendTime,
		MsgStatus:  domain.StatusSent,
		MsgType:    cmd.MsgType,
	}
	f.sent = append(f.sent, &msg)
	return msg, false, nil
}

func (f *fakeRepo) ListSessions(context.Context, domain.SessionQuery) ([]domain.Session, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) FindSession(context.Context, string, ctype.ID, bool) (domain.Session, error) {
	// 默认没有会话：这一组用例观察的是"陌生人"分支的拒绝回执；
	// 既有会话不参与判定由 chat/app 的用例覆盖。
	return domain.Session{}, apperr.New(apperr.CodeChatSessionNotFound)
}

func (f *fakeRepo) ListMessages(context.Context, domain.MessageQuery) (domain.MessageQueryResult, error) {
	return domain.MessageQueryResult{}, nil
}

func (f *fakeRepo) UserDeletedAt(context.Context, ctype.ID, string, []ctype.ID) (map[ctype.ID]time.Time, error) {
	return nil, nil
}

func (f *fakeRepo) MarkRead(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) MarkReadSession(context.Context, ctype.ID, string) ([]domain.Message, error) {
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

// fakeStranger 是**有效陌生人判定**的结论（站点总闸 AND 接收方个人开关，
// 两级合成在装配层；#247 起带原因，适配器据此返回不同文案）。
type fakeStranger struct{ verdict domain.StrangerVerdict }

func (f fakeStranger) CheckStranger(context.Context, ctype.ID) (domain.StrangerVerdict, error) {
	return f.verdict, nil
}

// fakeTickets 返回固定票据对应的会话身份。
//
// 字段用 ctype.ID 而不是裸 uint64：R12 要求"ID 语义的字段必须用 ctype.ID"，
// 裸整型会让契约印成 integer，前端会丢雪花 ID 的精度。
type fakeTickets struct {
	user    ctype.ID
	session ctype.ID
}

func (fakeTickets) Store(context.Context, string, uint64, uint64, time.Duration) error { return nil }

func (f fakeTickets) Consume(context.Context, string) (uint64, uint64, bool, error) {
	return f.user.Uint64(), f.session.Uint64(), true, nil
}

type fakeAuth struct{}

func (fakeAuth) Authenticate(context.Context, string) (uint64, uint64, bool, bool) {
	return 0, 0, false, false
}

func (fakeAuth) SessionActive(context.Context, uint64, uint64) (bool, error) { return true, nil }

// newService 造一个"发送一律成功"的用例服务；relation/verdict 决定陌生人判定。
func newService(relation int8, verdict domain.StrangerVerdict) (*app.Service, *fakeRepo) {
	repo := &fakeRepo{}
	svc := app.New(app.Deps{
		Read: repo, Write: repo, Relations: fakeRelations{relation: relation}, Users: fakeUsers{},
		Stranger: fakeStranger{verdict: verdict}, Limiter: permitLimiter{}, Blocks: permitBlocks{},
	})
	return svc, repo
}

// newEngine 挂上发送与 WS 两个入口，并注入 uid=1 的登录态。
func newEngine(svc *app.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:    ctype.NewID(1),
			SessionID: ctype.NewID(1),
			Role:      enum.RoleUser,
		}})
	})
	handler := chathttp.NewHandler(svc, ws.NewHub(), fakeTickets{user: ctype.NewID(1), session: ctype.NewID(1)}, fakeAuth{}, nil)
	engine.POST("/chat/messages", handler.Send)
	engine.GET("/chat/ws", handler.Ws)
	return engine
}

// postMessage 走真实 HTTP 发送接口，返回解析后的响应壳。
func postMessage(t *testing.T, engine *gin.Engine, receiverID uint64, content string) res.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"receiver_id":   ctype.NewID(receiverID),
		"msg_type":      1,
		"content":       content,
		"client_msg_id": "client-key-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/messages", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var envelope res.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是 JSON 壳: %v，原文 %s", err, rec.Body.String())
	}
	return envelope
}

// TestSend_自己发给自己成功时也回执 自己发给自己：发送成功后必须拿到回执
// （code=0 且带回落库后的消息），发送者才知道该刷新。
func TestSend_自己发给自己成功时也回执(t *testing.T) {
	svc, repo := newService(1, domain.StrangerBlockedBySite)
	engine := newEngine(svc)

	got := postMessage(t, engine, 1, "自言自语")
	if got.Code != res.SuccessCode {
		t.Fatalf("自己发给自己成功时应回成功回执，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d 条", len(repo.sent))
	}
	item, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("成功回执应带回消息实体，实际 %#v", got.Data)
	}
	if _, hasID := item["id"]; !hasID {
		t.Fatalf("回执里的消息必须带 id（前端要用它替换乐观消息），实际 %#v", item)
	}
}

// TestSend_陌生人被站点开关拒绝时回执说明站点策略 陌生人被站点开关拒绝时，
// 回执必须说明是站点策略（而不是"网络错误/账号受限"）。
func TestSend_陌生人被站点开关拒绝时回执说明站点策略(t *testing.T) {
	svc, repo := newService(1, domain.StrangerBlockedBySite) // relation=1 陌生人；站点开关关闭
	engine := newEngine(svc)

	got := postMessage(t, engine, 2, "你好")
	// 失败壳的具体码由 HTTP 响应层决定（业务错误统一走 FailServiceCode），
	// 这里钉住"非成功"，并把真正的判定交给文案：文案才是读者看到的东西。
	if got.Code == res.SuccessCode {
		t.Fatalf("被拒绝时不能回成功回执，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	// 读者侧显示的就是这一串文案。必须是既有错误码对应的那条明确原因，
	// 而不是随便一句"发送失败"。
	if got.Msg != apperr.Message(apperr.CodeChatStrangerOff) {
		t.Fatalf("回执文案应为 %q，实际 %q", apperr.Message(apperr.CodeChatStrangerOff), got.Msg)
	}
	if !strings.Contains(got.Msg, "站点") || !strings.Contains(got.Msg, "陌生人") {
		t.Fatalf("拒绝原因必须点明是站点陌生人私信开关，实际 %q", got.Msg)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
	}
}

// TestSend_陌生人被接收方开关拒绝时回执指向对方 #247：同是拒绝，原因在接收方
// 的个人开关上。回执文案必须提示"对方"的设置，不能沿用站点级那条去让人找
// 管理员 —— 管理员查站点总闸只会看到"已开启"。
func TestSend_陌生人被接收方开关拒绝时回执指向对方(t *testing.T) {
	svc, repo := newService(1, domain.StrangerBlockedByRecipient)
	engine := newEngine(svc)

	got := postMessage(t, engine, 2, "你好")
	if got.Code == res.SuccessCode {
		t.Fatalf("被拒绝时不能回成功回执，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if got.Msg != apperr.Message(apperr.CodeChatRecipientStrangerOff) {
		t.Fatalf("回执文案应为 %q，实际 %q", apperr.Message(apperr.CodeChatRecipientStrangerOff), got.Msg)
	}
	if !strings.Contains(got.Msg, "对方") {
		t.Fatalf("拒绝原因必须指向接收方的个人开关，实际 %q", got.Msg)
	}
	if strings.Contains(got.Msg, "管理员") {
		t.Fatalf("接收方级拒绝不该让用户去联系管理员，实际 %q", got.Msg)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
	}
}

// TestWs_上行消息统一提示改走HTTP（#201 后 WS 只做通知）。
//
// 旧客户端仍可能往 WS 发消息：必须明确告诉它该走哪个接口，
// 而不是静默吞掉（吞掉的症状是"点了发送没反应"）。
func TestWs_上行消息统一提示改走HTTP(t *testing.T) {
	svc, _ := newService(1, domain.StrangerAllowed)
	engine := newEngine(svc)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/chat/ws?ticket=t"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接 WS 失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	payload, _ := json.Marshal(map[string]any{
		"receiver_id": ctype.NewID(2),
		"msg_type":    1,
		"content":     "你好",
	})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("发送 WS 请求失败: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取 WS 回执失败: %v", err)
	}
	var envelope res.Response
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("回执不是响应壳: %v，原文 %s", err, data)
	}
	if envelope.Code != res.FailValidCode {
		t.Fatalf("WS 上行应回失败回执，实际 code=%d msg=%q", envelope.Code, envelope.Msg)
	}
	if !strings.Contains(envelope.Msg, "/chat/messages") {
		t.Fatalf("回执必须指向上行发送接口，实际 %q", envelope.Msg)
	}
}
