package app

import (
	"context"
	"encoding/json"

	authapp "myblogx/internal/auth/app"
	chatroot "myblogx/internal/chat"
	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
)

// WireChat 把私信模块装配进本进程。
//
// 装配本身已搬回模块内（internal/chat.New）：合成根只交出基础设施与跨模块
// 能力，不再逐个 new 私信的适配器，也不再把处理器塞进路由包的包级变量。
// 合成根仍是**跨模块适配器**（关注关系、用户偏好、认证）的唯一构造点。
//
// 连接池挂在 Infra 上（本进程装配结果），不是包级可变状态：
// "哪个进程能投私信通知"由 main 在启动中继时**显式传入**
// （见 WireOutboxRelay 的 OutboxWiringOptions），不由装配顺序反推。
func WireChat(infra *Infra) *chatroot.Module {
	if infra == nil {
		return nil
	}
	// 用户偏好端口必须先判空：typed-nil 赋给接口会变成非 nil 接口，
	// 陌生人判定就会在调用时 panic 而不是按站点级失败关闭。
	var userSettings chatroot.UserSettings
	if userService != nil {
		userSettings = userService
	}
	module := chatroot.New(
		infra.DB,
		infra.Redis,
		infra.Logger,
		infra.RuntimeSite,
		&chatRelationBridge{},
		userSettings,
		chatAuthBridge{svc: authService},
	)
	if module != nil {
		infra.ChatHub = module.Hub
	}
	return module
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

// chatRelationBridge 把 social 模块的关注关系适配成私信的使用方端口（#280）。
//
// 跨模块适配器留在合成根：私信模块只定义自己需要的 domain.RelationReader，
// 不 import social；social 也不需要知道私信存在。
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

// chatAuthBridge 用认证模块校验令牌与会话（跨模块适配器，留在合成根）。
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
