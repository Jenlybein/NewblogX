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

// WireChat 装配私信模块并注入路由处理器。
//
// 连接池挂在 Infra 上（本进程装配结果），不是包级可变状态：
// "哪个进程能投私信通知"由 main 在启动中继时**显式传入**
// （见 WireOutboxRelay 的 OutboxWiringOptions），不由装配顺序反推。
func WireChat(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	hub := ws.NewHub()
	infra.ChatHub = hub

	// 同一个仓储同时实现读端口与写端口；用例层分别注入，读写分离在接口层面成立。
	store := chatmysql.New(infra.DB)
	stranger := &strangerBridge{inner: infra.RuntimeSite}
	if userService != nil {
		stranger.users = userService
	}
	svc := chatapp.New(
		store,
		store,
		&chatRelationBridge{},
		&chatUserBridge{db: infra.DB},
		stranger,
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
//
// 连接池在**投递时**从 Infra 现读，而不是在注册时捕获：
// 这样即使装配顺序变成"先 WireOutboxRelay 后 WireChat"，订阅者拿到的仍是
// 最终连接池，不会因为注册那一刻是 nil 就静默丢通知。
func chatSubscriber(infra *Infra) outboxrelay.Subscriber {
	var logger chatdomain.Logger
	if infra != nil {
		logger = infra.Logger
	}
	return func(_ context.Context, e outboxrelay.Event) error {
		if infra == nil {
			return nil
		}
		hub := infra.ChatHub
		if hub == nil {
			return nil
		}
		switch e.EventName {
		case chatdomain.EventMessageSent:
			var notice chatdomain.SentNotice
			if err := json.Unmarshal([]byte(e.Payload), &notice); err != nil {
				warnBadPayload(logger, e, err)
				return nil
			}
			pushNotice(hub, notice.ReceiverID, notice)
		case chatdomain.EventMessagesRead:
			var read chatdomain.ReadEvent
			if err := json.Unmarshal([]byte(e.Payload), &read); err != nil {
				warnBadPayload(logger, e, err)
				return nil
			}
			// 已读回执推给发送方，线上形状与既有 ReadPush 一致。
			pushNotice(hub, read.SenderID, read.ToPush())
		}
		return nil
	}
}

// pushNotice 给某个用户的所有在线连接推一条通知帧。
//
// 推送成功数为 0 不是失败：对方不在线时通知没有意义（消息已经落库，
// 下次打开会话仍能拉到），重试只会白白进死信。
func pushNotice(hub *ws.Hub, userID ctype.ID, payload any) {
	if hub == nil || userID.IsZero() {
		return
	}
	hub.PushJSON(userID.Uint64(), res.Response{
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

// strangerUserSettings 是 user 模块暴露的偏好读端口（user/app.SettingsPort）。
//
// 装配层只依赖"接收方愿不愿意收陌生人私信"这一条读能力，不碰 user 仓储、
// 也不写 SQL：个人开关的真相留在用户模块里（#243）。
type strangerUserSettings interface {
	StrangerChatEnabled(ctx context.Context, id ctype.ID) (bool, error)
}

// strangerBridge 把两级开关合成一个结论（#243）。
//
//	有效判决 = 站点总闸(site.chat.stranger_msg_enabled)
//	           AND 接收方个人开关(user_conf_models.stranger_chat_enabled，默认开)
//
// 站点一级来自运行时站点配置，每次现读缓存，后台改完保存后立即生效；
// 接收方一级走 user 模块读端口，每次现读，账号设置改完立即生效。
// 任一级关闭即拒绝；站点关闭时不再多查一次用户偏好。
type strangerBridge struct {
	inner *sitecfg.RuntimeConfigService
	users strangerUserSettings
}

// StrangerEnabled 实现 domain.StrangerPolicy。
func (b *strangerBridge) StrangerEnabled(ctx context.Context, receiverID ctype.ID) (bool, error) {
	if b.inner == nil {
		// 未装配运行时配置时按"关"处理（与默认值一致，失败关闭）。
		return false, nil
	}
	if !b.inner.GetRuntimeSite().Chat.StrangerMsgEnabled {
		return false, nil
	}
	if b.users == nil {
		// 用户模块未装配时无法确认接收方意愿：按"关"处理，失败关闭。
		return false, nil
	}
	return b.users.StrangerChatEnabled(ctx, receiverID)
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
