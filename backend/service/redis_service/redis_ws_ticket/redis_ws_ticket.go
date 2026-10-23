// WebSocket 连接鉴权票据模块
// 作用：生成一次性临时票据，用于 WebSocket 建立连接前的鉴权校验
package redis_ws_ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"myblogx/models/ctype"
	"myblogx/service/redis_service"
)

// chatWSTicketPrefix Redis 中存储 WebSocket 票据的 key 前缀
const chatWSTicketPrefix = "chat:ws:ticket:"

// TicketPayload 票据携带的用户核心信息
type TicketPayload struct {
	UserID    ctype.ID `json:"user_id"`    // 用户ID
	SessionID ctype.ID `json:"session_id"` // 会话ID
}

// Store 存储票据到 Redis
func Store(deps redis_service.Deps, ticket string, payload TicketPayload, ttl time.Duration) error {
	//	ticket - 一次性随机票据字符串
	//	payload - 票据携带的用户信息
	//	ttl - 过期时间（一次性票据建议设置较短，如 10 秒）

	// 将用户信息序列化为 JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// 存入 Redis，设置过期时间
	if deps.Client == nil {
		return fmt.Errorf("redis 未初始化")
	}
	return deps.Client.Set(context.Background(), ticketKey(ticket), data, ttl).Err()
}

// Consume 消费票据（一次性使用，获取后立即删除）
// 成功：返回票据中的用户信息
// 失败：返回错误（票据不存在/已过期/非法）
func Consume(deps redis_service.Deps, ticket string) (*TicketPayload, error) {
	if deps.Client == nil {
		return nil, fmt.Errorf("redis 未初始化")
	}

	// GetDel：获取并立即删除 key，保证票据只能使用一次
	data, err := deps.Client.GetDel(context.Background(), ticketKey(ticket)).Bytes()
	if err != nil {
		return nil, err
	}

	// 反序列化 JSON 数据
	var payload TicketPayload
	if err = json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

// ticketKey 拼接 Redis 完整 key
func ticketKey(ticket string) string {
	return chatWSTicketPrefix + ticket
}
