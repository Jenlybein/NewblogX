// Package http 是私信模块的 HTTP + WebSocket 适配器。
package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
	"myblogx/internal/platform/safego"
	"myblogx/utils/jwts"
)

const (
	wsReadLimit  = 4 * 1024
	wsPongWait   = 60 * time.Second
	wsWriteWait  = 10 * time.Second
	wsPingPeriod = wsPongWait * 9 / 10
)

// TicketStore 是 WS 票据存取端口。
type TicketStore interface {
	Store(ctx context.Context, ticket string, userID, sessionID uint64, ttl time.Duration) error
	Consume(ctx context.Context, ticket string) (userID, sessionID uint64, ok bool, err error)
}

// Authenticator 是访问令牌/会话校验端口。
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (userID, sessionID uint64, admin bool, ok bool)
	SessionActive(ctx context.Context, userID, sessionID uint64) (bool, error)
}

// Handler 是私信处理器。
type Handler struct {
	svc     *app.Service
	hub     *ws.Hub
	tickets TicketStore
	auth    Authenticator
	logger  domain.Logger
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service, hub *ws.Hub, tickets TicketStore, auth Authenticator, logger domain.Logger) *Handler {
	if svc == nil {
		panic("chat/http: 用例服务未注入")
	}
	if hub == nil {
		panic("chat/http: 在线连接池未注入")
	}
	return &Handler{svc: svc, hub: hub, tickets: tickets, auth: auth, logger: logger}
}

func viewer(c *gin.Context) (ctype.ID, bool) {
	cl := jwts.GetClaimsByGin(c)
	if cl == nil {
		res.FailWithMsg("请先登录", c)
		return "", false
	}
	return cl.UserID, cl.IsAdmin()
}

// ── 请求/响应 ──

// SessionListQuery 会话列表查询。
type SessionListQuery struct {
	Page   int      `form:"page"`
	Limit  int      `form:"limit"`
	UserID ctype.ID `form:"user_id"`
	Type   int8     `form:"type" binding:"required,oneof=1 2" enums:"1,2"`
}

// MessageListQuery 消息列表查询。
type MessageListQuery struct {
	Page      int      `form:"page"`
	Limit     int      `form:"limit"`
	SessionID string   `form:"session_id" binding:"required"`
	UserID    ctype.ID `form:"user_id"`
	Type      int8     `form:"type" binding:"required,oneof=1 2" enums:"1,2"`
}

// IDListRequest 是一批消息 ID。
type IDListRequest struct {
	MsgIDList []uint64 `json:"msg_id_list" binding:"required"`
}

// SessionIDListRequest 是一批会话 ID。
type SessionIDListRequest struct {
	SessionIDList []string `json:"session_id_list" binding:"required"`
}

// ChatRequest 是 WS 上行消息。
type ChatRequest struct {
	ReceiverID ctype.ID `json:"receiver_id" binding:"required"`
	MsgType    int8     `json:"msg_type" binding:"required,oneof=1 2 7" enums:"1,2,7"`
	Content    string   `json:"content" binding:"required"`
}

// Sessions 会话列表。
// @Summary 会话列表
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]domain.SessionItem,count=int},msg=string}
// @Router /chat/sessions [get]
func (h *Handler) Sessions(c *gin.Context) {
	var q SessionListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, admin := viewer(c)
	target := q.UserID
	switch q.Type {
	case 1:
		target = uid
	case 2:
		if !admin {
			res.FailWithError(apperr.New(apperr.CodeChatNoPermission), c)
			return
		}
		if target.IsZero() {
			res.FailWithMsg("user_id 不能为 0", c)
			return
		}
	}
	list, count, err := h.svc.Sessions(c.Request.Context(), uid, target, admin, domain.Page{Page: q.Page, Limit: q.Limit})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, count, c)
}

// Messages 消息列表。
// @Summary 会话消息列表
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]domain.MessageItem,count=int},msg=string}
// @Router /chat/messages [get]
func (h *Handler) Messages(c *gin.Context) {
	var q MessageListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, admin := viewer(c)
	target := q.UserID
	switch q.Type {
	case 1:
		target = uid
	case 2:
		if !admin {
			res.FailWithError(apperr.New(apperr.CodeChatNoPermission), c)
			return
		}
		if target.IsZero() {
			res.FailWithMsg("user_id 不能为 0", c)
			return
		}
	}
	list, count, err := h.svc.Messages(c.Request.Context(), uid, target, q.SessionID, admin, domain.Page{Page: q.Page, Limit: q.Limit})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, count, c)
}

// Read 批量标记已读。
// @Summary 批量标记消息已读
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /chat/read [post]
func (h *Handler) Read(c *gin.Context) {
	var req IDListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, _ := viewer(c)
	ids := toIDs(req.MsgIDList)
	msgs, err := h.svc.MarkRead(c.Request.Context(), uid, ids)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	if len(msgs) == 0 {
		res.FailWithMsg("没有可标记已读的消息", c)
		return
	}

	now := time.Now()
	grouped := map[ctype.ID]map[string][]ctype.ID{}
	for _, msg := range msgs {
		sessions, ok := grouped[msg.SenderID]
		if !ok {
			sessions = map[string][]ctype.ID{}
			grouped[msg.SenderID] = sessions
		}
		sessions[msg.SessionID] = append(sessions[msg.SessionID], msg.ID)
	}
	for senderID, sessions := range grouped {
		for sessionID, msgIDs := range sessions {
			h.hub.PushJSON(senderID.Uint64(), domain.ReadPush{
				MsgType:   domain.TypeRead,
				SessionID: sessionID,
				ReaderID:  uid,
				MsgIDList: msgIDs,
				ReadAt:    now,
			})
		}
	}
	res.OkWithMsg(fmt.Sprintf("批量标记已读%d条消息", len(msgs)), c)
}

// DeleteMessages 删除消息。
// @Summary 删除消息
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /chat/messages [delete]
func (h *Handler) DeleteMessages(c *gin.Context) {
	var req IDListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, _ := viewer(c)
	count, err := h.svc.DeleteMessages(c.Request.Context(), uid, toIDs(req.MsgIDList))
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg(fmt.Sprintf("请求删除消息%d个，成功%d条", len(req.MsgIDList), count), c)
}

// DeleteSessions 删除会话。
// @Summary 删除会话
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /chat/sessions [delete]
func (h *Handler) DeleteSessions(c *gin.Context) {
	var req SessionIDListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, _ := viewer(c)
	count, err := h.svc.DeleteSessions(c.Request.Context(), uid, req.SessionIDList)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg(fmt.Sprintf("请求删除会话%d个，成功%d条", len(req.SessionIDList), count), c)
}

// WsTicket 生成聊天票据。
// @Summary 获取聊天 WebSocket 票据
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /chat/ws-ticket [get]
func (h *Handler) WsTicket(c *gin.Context) {
	cl := jwts.GetClaimsByGin(c)
	if cl == nil {
		res.FailWithMsg("请先登录", c)
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		res.FailWithMsg("生成 ws 票据失败", c)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)
	if err := h.tickets.Store(c.Request.Context(), ticket, cl.UserID.Uint64(), cl.SessionID.Uint64(), time.Minute); err != nil {
		res.FailWithMsg("生成 ws 票据失败", c)
		return
	}
	res.OkWithData(gin.H{"ticket": ticket}, c)
}

// Ws 聊天长连接。
// @Summary 聊天 WebSocket 长连接
// @Tags chat
// @Produce json
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /chat/ws [get]
func (h *Handler) Ws(c *gin.Context) {
	ctx := c.Request.Context()
	userID, _, ok := h.authenticateWS(ctx, c)
	if !ok {
		return
	}

	raw, err := ws.Upgrade(c.Writer, c.Request, middleware.IsAllowedOriginRequest)
	if err != nil {
		return
	}
	conn := ws.NewConn(userID.Uint64(), raw)
	h.hub.Register(conn)

	defer func() {
		h.hub.Unregister(conn)
		_ = conn.Close()
	}()

	conn.SetReadLimit(wsReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(wsPongWait)) })

	done := make(chan struct{})
	defer close(done)
	safego.Go(func(r any, stack []byte) {
		if h.logger != nil {
			h.logger.Errorf("聊天心跳协程 panic: %v\n%s", r, stack)
		}
	}, func() { _ = conn.RunPingLoop(done, wsPingPeriod, wsWriteWait) })

	for {
		msgType, content, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}
		var req ChatRequest
		if err := json.Unmarshal(content, &req); err != nil {
			if err := res.SendConnFailWithMsg("消息格式错误", conn, wsWriteWait); err != nil {
				return
			}
			continue
		}

		msg, err := h.svc.Send(ctx, userID, req.ReceiverID, domain.MsgType(req.MsgType), req.Content)
		if err != nil {
			if werr := res.SendConnFailWithMsg(err.Error(), conn, wsWriteWait); werr != nil {
				return
			}
			continue
		}

		item := domain.MessageItem{
			ID:         msg.ID,
			SenderID:   msg.SenderID,
			ReceiverID: msg.ReceiverID,
			SessionID:  msg.SessionID,
			Content:    msg.Content,
			SendTime:   msg.SendTime,
			MsgStatus:  msg.MsgStatus,
			MsgType:    msg.MsgType,
			IsSelf:     false,
			IsRead:     false,
		}
		if req.ReceiverID.Uint64() == userID.Uint64() {
			if success := h.hub.PushJSON(msg.ReceiverID.Uint64(), item); success == 0 {
				if err := res.SendConnOkWithMsg("给自己发送消息", conn, wsWriteWait); err != nil {
					return
				}
			}
			continue
		}
		if success := h.hub.PushJSON(msg.ReceiverID.Uint64(), item); success == 0 {
			if err := res.SendConnFailWithMsg("接收人不在线", conn, wsWriteWait); err != nil {
				return
			}
		}
	}
}

// authenticateWS 先试 Header token，再试 ticket。
func (h *Handler) authenticateWS(ctx context.Context, c *gin.Context) (ctype.ID, uint64, bool) {
	if token := jwts.GetTokenByGin(c); token != "" && h.auth != nil {
		if uid, sid, _, ok := h.auth.Authenticate(ctx, token); ok {
			return ctype.NewID(uid), sid, true
		}
	}
	ticket := c.Query("ticket")
	if ticket == "" || h.auth == nil {
		res.FailWithError(apperr.New(apperr.CodeAuthRequired), c)
		return "", 0, false
	}
	uid, sid, ok, err := h.tickets.Consume(ctx, ticket)
	if err != nil || !ok {
		res.FailWithError(apperr.New(apperr.CodeUserTokenInvalid), c)
		return "", 0, false
	}
	active, err := h.auth.SessionActive(ctx, uid, sid)
	if err != nil || !active {
		res.FailWithError(apperr.New(apperr.CodeUserSessionExpired), c)
		return "", 0, false
	}
	return ctype.NewID(uid), sid, true
}

func toIDs(ids []uint64) []ctype.ID {
	out := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		out = append(out, ctype.NewID(id))
	}
	return out
}
