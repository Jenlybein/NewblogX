package redis_chat

import (
	"context"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// chatUserMinuteLimit 表示同一发送用户在滑动 60 秒窗口内最多允许发送 60 条消息。
	chatUserMinuteLimit = 60
	// chatSessionMinuteLimit 表示同一会话在滑动 60 秒窗口内最多允许发送 30 条消息。
	chatSessionMinuteLimit = 30
	// chatMinuteWindow 是分钟级限流的滑动窗口长度。
	chatMinuteWindow = 60 * time.Second
	// chatMinuteTTL 是分钟级限流 key 的保留时间，略大于窗口本身，避免频繁重建 key。
	chatMinuteTTL = 120 * time.Second
)

var (
	// chatMinuteReserveScript 在一个 Lua 脚本中同时完成清理过期成员、统计窗口内数量和写入新消息，
	// 保证用户级与会话级分钟限流的判断和预占是原子的。
	chatMinuteReserveScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local userLimit = tonumber(ARGV[3])
local sessionLimit = tonumber(ARGV[4])
local member = ARGV[5]
local ttl = tonumber(ARGV[6])

-- 清理过期数据：ZSet中超出滑动时间窗口的成员
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now - window)
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now - window)

-- 统计窗口内数量，判断是否达到限制
local userCount = redis.call("ZCARD", KEYS[1])
if userCount >= userLimit then
	return {0, "user"}
end

local sessionCount = redis.call("ZCARD", KEYS[2])
if sessionCount >= sessionLimit then
	return {0, "session"}
end

-- 写入新消息
redis.call("ZADD", KEYS[1], now, member)
redis.call("ZADD", KEYS[2], now, member)
redis.call("PEXPIRE", KEYS[1], ttl)
redis.call("PEXPIRE", KEYS[2], ttl)

return {1, "ok"}
`)
	// chatMinuteReleaseScript 用于在消息最终未落库时撤销分钟级预占，避免失败请求占用额度。
	chatMinuteReleaseScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("ZREM", KEYS[2], ARGV[1])
return 1
`)
	chatRateSeq int64
)

// MinuteReservation 表示一次分钟级限流预占。
// 上层在消息真正落库失败时可以调用 Release 回滚本次占用。
type MinuteReservation struct {
	userKey    string
	sessionKey string
	member     string
}

// Release 撤销一次分钟级预占，用于消息最终未落库时回滚本次占用。
func (r *MinuteReservation) Release(deps redis_service.Deps) error {
	client := deps.Client
	if r == nil || client == nil {
		return nil
	}

	_, err := chatMinuteReleaseScript.Run(context.Background(), client,
		[]string{r.userKey, r.sessionKey},
		r.member,
	).Result()
	return err
}

// ReserveChatMinuteRate 尝试为一条待发送消息预占分钟级额度。
// 返回值说明：
// 1. reservation 非空：预占成功；
// 2. reservation 为空且 limitedBy 非空：被限流；
// 3. err 非空：Redis 执行异常。
func ReserveChatMinuteRate(deps redis_service.Deps, senderID ctype.ID, sessionID string, now time.Time) (*MinuteReservation, string, error) {
	client := deps.Client
	if client == nil {
		return nil, "", fmt.Errorf("redis 未初始化")
	}

	// 生成本次消息在滑动窗口内的唯一成员值
	member := buildChatRateMember(senderID, now)
	userKey := chatUserMinuteKey(senderID)
	sessionKey := chatSessionMinuteKey(sessionID)

	// 在一个 Lua 脚本中同时完成清理过期成员、统计窗口内数量和写入新消息
	result, err := chatMinuteReserveScript.Run(context.Background(), client, []string{userKey, sessionKey},
		now.UnixMilli(),
		chatMinuteWindow.Milliseconds(),
		chatUserMinuteLimit,
		chatSessionMinuteLimit,
		member,
		chatMinuteTTL.Milliseconds(),
	).Result()
	if err != nil {
		return nil, "", err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return nil, "", fmt.Errorf("聊天分钟限流返回结果异常: %v", result)
	}
	allowed, ok := values[0].(int64)
	if !ok {
		return nil, "", fmt.Errorf("聊天分钟限流状态异常: %v", values[0])
	}
	reason, ok := values[1].(string)
	if !ok {
		return nil, "", fmt.Errorf("聊天分钟限流原因异常: %v", values[1])
	}
	if allowed == 0 {
		return nil, reason, nil
	}

	return &MinuteReservation{
		userKey:    userKey,
		sessionKey: sessionKey,
		member:     member,
	}, "", nil
}

// buildChatRateMember 生成本次消息在滑动窗口内的唯一成员值。
func buildChatRateMember(senderID ctype.ID, now time.Time) string {
	// atomic 包提供原子操作函数 AddInt64，专门用于对 int64 类型变量执行原子加法
	seq := atomic.AddInt64(&chatRateSeq, 1)
	// 根据 senderID 和时间戳生成唯一成员值
	return senderID.String() + ":" + strconv.FormatInt(now.UnixNano(), 10) + ":" + strconv.FormatInt(seq, 10)
}

// chatUserMinuteKey 返回用户级分钟限流 key。
func chatUserMinuteKey(senderID ctype.ID) string {
	return "chat:rate:user:" + senderID.String()
}

// chatSessionMinuteKey 返回会话级分钟限流 key。
func chatSessionMinuteKey(sessionID string) string {
	return "chat:rate:session:" + sessionID
}
