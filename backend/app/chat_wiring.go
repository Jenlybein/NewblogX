package app

import (
	"context"

	authapp "myblogx/internal/auth/app"
	chathttp "myblogx/internal/chat/adapter/http"
	chatmysql "myblogx/internal/chat/adapter/mysql"
	chatredis "myblogx/internal/chat/adapter/redis"
	chatapp "myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/http/ws"
	"myblogx/internal/platform/readmodel"
	redis_service "myblogx/internal/platform/redis"

	"gorm.io/gorm"
)

// WireChat 装配私信模块并注入路由处理器。
func WireChat(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	hub := ws.NewHub()

	svc := chatapp.New(
		chatmysql.New(infra.DB),
		&chatRelationBridge{},
		&chatUserBridge{db: infra.DB},
		&strangerBridge{db: infra.DB},
		chatredis.NewLimiter(redisOptions),
		chatHub{inner: hub},
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
func (b *chatUserBridge) Displays(_ context.Context, ids []ctype.ID) (map[ctype.ID]domain.Display, error) {
	raw, err := readmodel.LoadUserDisplayMap(b.db, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[ctype.ID]domain.Display, len(raw))
	for id, item := range raw {
		out[id] = domain.Display{Nickname: item.Nickname, Avatar: item.Avatar}
	}
	return out, nil
}

// strangerConfRow 对应 user_conf_models 的陌生人私信开关。
type strangerConfRow struct {
	UserID              ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	StrangerChatEnabled bool
}

// TableName 固定表名。
func (strangerConfRow) TableName() string { return "user_conf_models" }

// strangerBridge 读"是否接收陌生人私信"。
type strangerBridge struct {
	db *gorm.DB
}

// StrangerEnabled 实现 domain.StrangerPolicy。
func (b *strangerBridge) StrangerEnabled(ctx context.Context, userID ctype.ID) (bool, error) {
	var row strangerConfRow
	if err := b.db.WithContext(ctx).Select("user_id", "stranger_chat_enabled").
		Take(&row, "user_id = ?", userID.Uint64()).Error; err != nil {
		// 老账号没有 conf 行时按"未开启"处理，与旧实现一致。
		return false, nil
	}
	return row.StrangerChatEnabled, nil
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
	return principal.UserID.Uint64(), principal.SessionID.Uint64(), principal.Role == int8(enum.RoleAdmin), true
}

// SessionActive 实现 chathttp.Authenticator（票据换会话时校验会话仍有效）。
func (b chatAuthBridge) SessionActive(ctx context.Context, userID, sessionID uint64) (bool, error) {
	if b.svc == nil {
		return false, nil
	}
	sessions, err := b.svc.ListSessions(ctx, ctype.NewID(userID))
	if err != nil {
		return false, err
	}
	for _, session := range sessions {
		if session.ID.Uint64() == sessionID {
			return true, nil
		}
	}
	return false, nil
}

// chatHub 把平台在线池适配成 chat 的 domain.Hub（类型用 ctype.ID）。
type chatHub struct {
	inner *ws.Hub
}

// IsOnline 实现 domain.Hub。
func (h chatHub) IsOnline(userID ctype.ID) bool { return h.inner.IsOnline(userID.Uint64()) }

// PushJSON 实现 domain.Hub。
func (h chatHub) PushJSON(userID ctype.ID, v any) int { return h.inner.PushJSON(userID.Uint64(), v) }
