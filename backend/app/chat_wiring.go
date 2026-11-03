package app

import (
	"context"
	"encoding/json"

	authapp "myblogx/internal/auth/app"
	chathttp "myblogx/internal/chat/adapter/http"
	chatmysql "myblogx/internal/chat/adapter/mysql"
	chatredis "myblogx/internal/chat/adapter/redis"
	chatapp "myblogx/internal/chat/app"
	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/cdc/outboxrelay"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/http/ws"
	"myblogx/internal/platform/readmodel"
	redis_service "myblogx/internal/platform/redis"

	"gorm.io/gorm"
)

// chatPushHub 是私信 WS 通知用的进程内连接池。
//
// 由 WireChat 注入。只有真正持有连接池的进程（-role=api / -role=all）才有它；
// 其他角色的 outbox 中继必须跳过私信事件（见 relayOptions 的 SkipEventNames），
// 否则会在没有连接的进程里把事件"投递成功"并标记 done，通知就永久丢了。
var chatPushHub *ws.Hub

// WireChat 装配私信模块并注入路由处理器。
func WireChat(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	hub := ws.NewHub()
	chatPushHub = hub

	// 同一个仓储同时实现读端口与写端口；用例层分别注入，读写分离在接口层面成立。
	store := chatmysql.New(infra.DB)
	svc := chatapp.New(
		store,
		store,
		&chatRelationBridge{},
		&chatUserBridge{db: infra.DB},
		&strangerBridge{inner: infra.RuntimeSite},
		chatredis.NewLimiter(redisOptions),
		infra.Logger,
	)

	handler := chathttp.NewHandler(
		svc,
		hub,
		chatredis.NewTickets(redisOptions),
		chatAuthBridge{svc: authService},
		infra.Logger,
	)
	router.SetChatHandler(handler)
}

// chatSubscriber 把私信发件箱事件投递成 WebSocket 通知。
//
// 为什么是通知而不是"把消息推给对端"：HTTP 落库才是唯一真相（断网/断 WS 也能发出），
// WS 只负责"有新消息了"与"对方已读了"这两个即时信号；接收方收到通知后
// 自己走 HTTP 刷新会话、消息与未读数。
//
// 只做通知、不落库，因此天然幂等：中继重复投递只会多一次刷新信号。
func chatSubscriber(logger chatdomain.Logger) outboxrelay.Subscriber {
	return func(_ context.Context, e outboxrelay.Event) error {
		if chatPushHub == nil {
			return nil
		}
		switch e.EventName {
		case chatdomain.EventMessageSent:
			var notice chatdomain.SentNotice
			if err := json.Unmarshal([]byte(e.Payload), &notice); err != nil {
				warnBadPayload(logger, e, err)
				return nil
			}
			pushNotice(notice.ReceiverID, notice)
		case chatdomain.EventMessagesRead:
			var read chatdomain.ReadEvent
			if err := json.Unmarshal([]byte(e.Payload), &read); err != nil {
				warnBadPayload(logger, e, err)
				return nil
			}
			// 已读回执推给发送方，线上形状与既有 ReadPush 一致。
			pushNotice(read.SenderID, read.ToPush())
		}
		return nil
	}
}

// pushNotice 给某个用户的所有在线连接推一条通知帧。
//
// 推送成功数为 0 不是失败：对方不在线时通知没有意义（消息已经落库，
// 下次打开会话仍能拉到），重试只会白白进死信。
func pushNotice(userID ctype.ID, payload any) {
	if chatPushHub == nil || userID.IsZero() {
		return
	}
	chatPushHub.PushJSON(userID.Uint64(), res.Response{
		Code: res.SuccessCode,
		Data: payload,
		Msg:  "成功",
	})
}

// warnBadPayload 记录载荷解析失败；这类事件重投也不会成功，直接丢弃。
func warnBadPayload(logger chatdomain.Logger, e outboxrelay.Event, err error) {
	if logger == nil {
		return
	}
	logger.Warnf("私信发件箱事件载荷无法解析，已丢弃: id=%s name=%s err=%v", e.ID, e.EventName, err)
}

// chatRelationBridge 读关注关系（来自 social 模块）。
type chatRelationBridge struct{}

// Batch 实现 domain.RelationReader。
func (b *chatRelationBridge) Batch(ctx context.Context, viewer ctype.ID, ids []ctype.ID) (map[ctype.ID]int8, error) {
	out := make(map[ctype.ID]int8, len(ids))
	if socialService == nil || viewer.IsZero() {
		for _, id := range ids {
			out[id] = 1
		}
		return out, nil
	}
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.Uint64())
	}
	got, err := socialService.RelationBatch(ctx, viewer.Uint64(), raw)
	if err != nil {
		return nil, err
	}
	for id, rel := range got {
		out[ctype.NewID(id)] = rel.Int8()
	}
	return out, nil
}

// chatUserBridge 读用户展示快照。
type chatUserBridge struct {
	db *gorm.DB
}

// Displays 实现 domain.UserDirectory。
func (b *chatUserBridge) Displays(_ context.Context, ids []ctype.ID) (map[ctype.ID]chatdomain.Display, error) {
	raw, err := readmodel.LoadUserDisplayMap(b.db, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[ctype.ID]chatdomain.Display, len(raw))
	for id, item := range raw {
		out[id] = chatdomain.Display{Nickname: item.Nickname, Avatar: item.Avatar}
	}
	return out, nil
}

// strangerBridge 读站点级"是否允许陌生人发私信"开关。
//
// 这个值来自运行时站点配置（site.chat.stranger_msg_enabled），每次现读缓存，
// 所以后台改完保存后立即生效；不再读 user_conf_models 的按人开关。
type strangerBridge struct {
	inner *sitecfg.RuntimeConfigService
}

// StrangerEnabled 实现 domain.StrangerPolicy。
func (b *strangerBridge) StrangerEnabled(context.Context) (bool, error) {
	if b.inner == nil {
		// 未装配运行时配置时按"关"处理（与默认值一致，失败关闭）。
		return false, nil
	}
	return b.inner.GetRuntimeSite().Chat.StrangerMsgEnabled, nil
}

// chatAuthBridge 用认证模块校验令牌与会话。
type chatAuthBridge struct {
	svc *authapp.Service
}

// Authenticate 实现 chathttp.Authenticator。
func (b chatAuthBridge) Authenticate(ctx context.Context, token string) (uint64, uint64, bool, bool) {
	if b.svc == nil {
		return 0, 0, false, false
	}
	principal, err := b.svc.Authenticate(ctx, token)
	if err != nil {
		return 0, 0, false, false
	}
	// 与中间件同一条规则：受限访客 / 未知角色按未登录（否则 WS 直接读 header
	// 令牌就能绕过 AuthMiddleware，guest 仍能收发私信）。
	if err := requireWritablePrincipal(principal); err != nil {
		return 0, 0, false, false
	}
	return principal.UserID.Uint64(), principal.SessionID.Uint64(), principal.Role == int8(enum.RoleAdmin), true
}

// SessionActive 实现 chathttp.Authenticator（票据换会话时校验会话仍有效）。
func (b chatAuthBridge) SessionActive(ctx context.Context, userID, sessionID uint64) (bool, error) {
	if b.svc == nil {
		return false, nil
	}
	// 按会话 ID 精确校验，而不是从"按设备聚合"的列表里找——
	// 同设备的非代表会话不会出现在聚合结果中，拿它判断会误判失效。
	return b.svc.SessionActive(ctx, userID, sessionID)
}
