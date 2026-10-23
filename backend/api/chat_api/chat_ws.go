package chat_api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum/chat_msg_enum"
	"myblogx/service/chat_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_ws_ticket"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	chatWSReadBufferSize  = 1024
	chatWSWriteBufferSize = 1024
	chatWSReadLimit       = 4 * 1024
	chatWSPongWait        = 60 * time.Second
	chatWSWriteWait       = 10 * time.Second
	chatWSPingPeriod      = chatWSPongWait * 9 / 10
)

var chatWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  chatWSReadBufferSize,
	WriteBufferSize: chatWSWriteBufferSize,
	CheckOrigin:     middleware.IsAllowedOriginRequest,
}

// ChatWsTicketView 生成聊天票据
func (h ChatApi) ChatWsTicketView(c *gin.Context) {
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)
	claims := jwts.MustGetClaimsByGin(c)
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		res.FailWithMsg("生成 ws 票据失败", c)
		return
	}

	ticket := base64.RawURLEncoding.EncodeToString(raw)
	if err := redis_ws_ticket.Store(redisDeps, ticket, redis_ws_ticket.TicketPayload{
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
	}, time.Minute); err != nil {
		res.FailWithMsg("生成 ws 票据失败", c)
		return
	}

	res.OkWithData(gin.H{"ticket": ticket}, c)
}

// ChatWsView 处理聊天 WebSocket 长连接。
func (h ChatApi) ChatWsView(c *gin.Context) {
	logger := h.App.Logger
	db := h.App.DB
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)
	authenticator := user_service.NewAuthenticator(
		h.App.DB,
		h.App.Logger,
		h.App.JWT,
		redisDeps,
	)
	// 尝试从 Header 中的 token 中获取用户
	var authResult *user_service.AuthResult
	token := jwts.GetTokenByGin(c)
	if token != "" {
		if result, err := authenticator.AuthenticateAccessToken(token); err == nil {
			authResult = result
		}
	}
	if authResult == nil {
		// Header 中未携带 token，尝试从 query 中获取票据，再尝试从 redis 中获取用户数据
		ticket := c.Query("ticket")
		if ticket == "" {
			res.FailWithMsg(user_service.ErrAuthRequired.Error(), c)
			return
		}

		payload, err := redis_ws_ticket.Consume(redisDeps, ticket)
		if err != nil {
			res.FailWithMsg(user_service.ErrAuthInvalid.Error(), c)
			return
		}
		authResult, err = authenticator.AuthenticateSession(payload.UserID, payload.SessionID)
		if err != nil {
			res.FailWithMsg(err.Error(), c)
			return
		}
	}
	claims := authResult.Claims

	// 升级成 ws 连接
	rawConn, err := chatWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf("升级聊天 WebSocket 连接失败: 用户ID=%d 错误=%v", claims.UserID, err)
		return
	}

	// 注册聊天 ws 连接
	conn := chat_service.NewChatConn(claims.UserID, rawConn)
	store := chat_service.GetOnlineUserStore()
	store.Register(conn)
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Warnf("关闭聊天 WebSocket 连接失败: 用户ID=%d 错误=%v", claims.UserID, err)
		}
		store.Unregister(conn)
		logger.Infof("聊天 WebSocket 已清理: 用户ID=%d 在线连接数=%d", claims.UserID, store.Count(claims.UserID))
	}()

	// 配置聊天 ws 连接
	configureChatWSConn(conn)
	logger.Infof("聊天 WebSocket 已连接: 用户ID=%d 在线连接数=%d", claims.UserID, store.Count(claims.UserID))

	// 心跳检测
	done := make(chan struct{})
	defer close(done)
	go func() {
		if err := conn.RunPingLoop(done, chatWSPingPeriod, chatWSWriteWait); err != nil {
			logger.Warnf("聊天 WebSocket 心跳失败: 用户ID=%d 错误=%v", conn.UserID, err)
		}
	}()

	for {
		// 读取消息
		msgType, msgContent, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				logger.Warnf("聊天 WebSocket 读取异常关闭: 用户ID=%d 错误=%v", claims.UserID, err)
			} else {
				logger.Infof("聊天 WebSocket 已断开: 用户ID=%d 原因=%v", claims.UserID, err)
			}
			return
		}

		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}

		// 解析消息
		var req ChatRequest
		if err := json.Unmarshal(msgContent, &req); err != nil {
			if err := res.SendConnFailWithMsg("消息格式错误", conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
			continue
		}

		// 检测接收人
		var revUser models.UserModel
		if err := db.Preload("UserConfModel").First(&revUser, req.ReceiverID).Error; err != nil {
			if err := res.SendConnFailWithMsg("接收人不存在", conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
			continue
		}

		// 检测发送权限
		reservation, err := chat_service.CheckAndReserveChatSend(db, redisDeps, claims.UserID, &revUser)
		if err != nil {
			if err := res.SendConnFailWithMsg(err.Error(), conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
			continue
		}

		// 处理消息进入数据库
		var msgModel *models.ChatMsgModel
		var msgErr error
		switch req.MsgType {
		case chat_msg_enum.MsgTypeText:
			msgModel, msgErr = chat_service.ToTextChat(db, logger, chat_service.ToTextChatRequest{
				SenderID:   claims.UserID,
				ReceiverID: req.ReceiverID,
				Text:       req.Content,
			})
		case chat_msg_enum.MsgTypeImage:
			msgModel, msgErr = chat_service.ToImageChat(db, logger, chat_service.ToImageChatRequest{
				SenderID:   claims.UserID,
				ReceiverID: req.ReceiverID,
				ImageURL:   req.Content,
			})
		case chat_msg_enum.MsgTypeMarkdown:
			msgModel, msgErr = chat_service.ToMarkdownChat(db, logger, chat_service.ToMarkdownChatRequest{
				SenderID:   claims.UserID,
				ReceiverID: req.ReceiverID,
				Markdown:   req.Content,
			})
		default:
			msgErr = errors.New("不支持的消息类型")
		}
		if msgErr != nil {
			_ = reservation.Rollback()
			if err := res.SendConnFailWithMsg("消息发送失败", conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
			logger.Warnf("聊天消息写入数据库失败: 用户ID=%d 错误=%v", claims.UserID, msgErr)
			continue
		}
		if err := reservation.Commit(); err != nil {
			logger.Warnf("聊天消息提交限流状态失败: 用户ID=%d 错误=%v", claims.UserID, err)
		}

		// 检测接收人是否在线
		if !store.IsOnline(req.ReceiverID) {
			if err := res.SendConnFailWithMsg("接收人不在线", conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
			continue
		}

		// 发送消息
		item := ChatMsgResponse{
			Content:    msgModel.Content,
			MsgType:    msgModel.MsgType,
			ID:         msgModel.ID,
			SendTime:   msgModel.SendTime,
			SenderID:   msgModel.SenderID,
			ReceiverID: msgModel.ReceiverID,
			SessionID:  msgModel.SessionID,
			IsSelf:     msgModel.SenderID == claims.UserID,
			IsRead:     false,
			MsgStatus:  msgModel.MsgStatus,
		}
		if req.ReceiverID == claims.UserID {
			if successCount := res.SendWsMsg(item, store, req.ReceiverID); successCount == 0 {
				if err := res.SendConnOkWithMsg("给自己发送消息", conn, chatWSWriteWait); err != nil {
					logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
					return
				}
			}
			continue
		}
		if successCount := res.SendWsMsg(item, store, req.ReceiverID); successCount == 0 {
			if err := res.SendConnFailWithMsg("消息发送失败", conn, chatWSWriteWait); err != nil {
				logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
				return
			}
		}

		// DEBUG：给自己也发一份
		// if err := res.SendConnOkWithData(item, conn, chatWSWriteWait); err != nil {
		// 	logger.Warnf("聊天 WebSocket 写入失败: 用户ID=%d 错误=%v", claims.UserID, err)
		// 	return
		// }
	}
}

// 初始化连接的读限制和 pong 续期逻辑。
func configureChatWSConn(conn *chat_service.ChatConn) {
	conn.SetReadLimit(chatWSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(chatWSPongWait))
	conn.SetPongHandler(func(h string) error {
		return conn.SetReadDeadline(time.Now().Add(chatWSPongWait))
	})
}
