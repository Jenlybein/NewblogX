// Package http 是私信模块的 HTTP + WebSocket 适配器。
package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
	"myblogx/internal/platform/safego"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
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

// ChatRequest 是 WS 上行消息（历史上 WS 曾是发送通道）。
//
// 现在 WS 只做通知（下行），发送一律走 POST /chat/messages；
// 保留类型名与字段是为了让旧客户端的载荷能被解析出可读错误，
// 而不是直接抛"消息格式错误"。
type ChatRequest struct {
	ReceiverID ctype.ID `json:"receiver_id" binding:"required"`
	MsgType    int8     `json:"msg_type" binding:"required,oneof=1 2 7" enums:"1,2,7"`
	Content    string   `json:"content" binding:"required"`
}

// SendMessageRequest 是 HTTP 发送消息的请求体。
type SendMessageRequest struct {
	ReceiverID ctype.ID `json:"receiver_id" binding:"required"`
	MsgType    int8     `json:"msg_type" binding:"required,oneof=1 2 7" enums:"1,2,7"`
	Content    string   `json:"content" binding:"required"`
	// ClientMsgID 是客户端生成的幂等键。
	//
	// 必须由客户端提供（同一次发送的多次重试要复用同一个值）：服务端把
	// （发送人, client_msg_id）映射到消息主键，重复提交返回第一次落库的消息，
	// 不产生第二条、不重复加未读。缺失时后端会退化成普通发送（不做幂等），
	// 所以这里要求必填，把幂等语义钉在契约上。
	ClientMsgID string `json:"client_msg_id" binding:"required,max=64"`
}

// Sessions 会话列表。
// @Summary 会话列表
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[domain.SessionItem]]
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
// @Success 200 {object} kernel.Envelope[kernel.ListData[domain.MessageItem]]
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

// Send 发送私信消息（HTTP 可靠落库）。
//
// 这是发送的唯一通道：消息先落库再返回，WebSocket 只负责把"有新消息 / 已读"
// 通知给对端。因此 WS 断开或网络抖动时消息照常发出，重连后对端刷新即可收到。
//
// 幂等：同一次发送的重试必须复用同一个 client_msg_id；重复提交返回第一次
// 落库的那条消息，不会产生两条，也不会把接收方未读再加一次。
//
// @Summary 发送私信消息
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SendMessageRequest true "发送参数"
// @Success 200 {object} kernel.Envelope[domain.MessageItem]
// @Router /chat/messages [post]
func (h *Handler) Send(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	uid, _ := viewer(c)
	msg, err := h.svc.Send(c.Request.Context(), uid, req.ReceiverID, domain.MsgType(req.MsgType), req.Content, req.ClientMsgID)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	// 响应体与消息列表里的行完全同形，前端可以直接用它替换乐观消息。
	res.OkWithData(domain.MessageItem{
		ID:         msg.ID,
		SenderID:   msg.SenderID,
		ReceiverID: msg.ReceiverID,
		SessionID:  msg.SessionID,
		Content:    msg.Content,
		SendTime:   msg.SendTime,
		MsgStatus:  msg.MsgStatus,
		MsgType:    msg.MsgType,
		IsSelf:     true,
		IsRead:     false,
	}, c)
}

// Read 批量标记已读。
// @Summary 批量标记消息已读
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
	// 已读回执不再在这里直推连接池：它随 MarkRead 一起写进发件箱，
	// 由中继的订阅者推给对应发送人（见 app/chat_wiring.go）。
	res.OkWithMsg(fmt.Sprintf("批量标记已读%d条消息", len(msgs)), c)
}

// DeleteMessages 删除消息。
// @Summary 删除消息
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
// @Success 200 {object} kernel.Envelope[WsTicketResponse]
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
	res.OkWithData(WsTicketResponse{Ticket: ticket}, c)
}

// Ws 聊天长连接。
// @Summary 聊天 WebSocket 长连接
// @Tags chat
// @Produce json
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
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
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		// WS 已降级为通知通道：发送一律走 HTTP 落库。这里明确告诉旧客户端
		// 该走哪里，而不是静默吞掉上行消息（吞掉的症状是"点了发送没反应"）。
		if err := res.SendConnFailWithMsg("私信发送请使用 POST /api/chat/messages（WebSocket 只做通知）", conn, wsWriteWait); err != nil {
			return
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

// WsTicketResponse 是 WebSocket 握手用的临时票据。
type WsTicketResponse struct {
	Ticket string `json:"ticket"`
}
