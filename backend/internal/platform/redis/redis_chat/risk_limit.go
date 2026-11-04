package redis_chat

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
)

// 发送限流口径（spec 269 / D-4，票 268，ADR-0026）。
//
// 参数由后台「站点 → 私信」现读下发，默认 **1 条 / 10 秒、99 条 / 自然日**。
// 它替换了旧的"用户 60/60s + 会话 30/60s + 自然周 1/3 条"三档：
//   - 周配额把陌生人压到每周 1 条过紧，且"对方回复才重置"的隐含规则难解释；
//   - 按日额度是可预期的额度，短窗口挡住连发刷屏。
//
// 两级都按 (sender, receiver) 计，方向独立；自己发给自己的会话同样受限
// （避免拿自聊当刷量通道）。窗口与额度由调用方传入，本文件不持有配置单例。
const (
	chatPairWindowLimit = 1
	chatPairDailyTTL    = 26 * time.Hour // 跨自然日边界时旧 key 自然过期
)

var (
	// chatPairReserveScript 在一个 Lua 脚本里原子的完成：清理过期窗口成员、
	// 检查 10 秒窗口与当日额度、写入两条记录并设 TTL。
	//
	// 返回 {1,"ok"} / {0,"window"} / {0,"daily"}；失败时调用方回滚。
	chatPairReserveScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local windowLimit = tonumber(ARGV[3])
local dailyLimit = tonumber(ARGV[4])
local dailyKey = ARGV[5]
local member = ARGV[6]
local windowTTL = tonumber(ARGV[7])
local dailyTTL = tonumber(ARGV[8])

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now - window)
local windowCount = redis.call("ZCARD", KEYS[1])
if windowCount >= windowLimit then
	return {0, "window"}
end

local dailyCount = tonumber(redis.call("GET", dailyKey) or "0")
if dailyCount >= dailyLimit then
	return {0, "daily"}
end

redis.call("ZADD", KEYS[1], now, member)
redis.call("PEXPIRE", KEYS[1], windowTTL)
redis.call("INCR", dailyKey)
redis.call("PEXPIRE", dailyKey, dailyTTL)
return {1, "ok"}
`)
	chatPairReleaseScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
local cur = tonumber(redis.call("GET", KEYS[2]) or "0")
if cur > 0 then
	redis.call("DECR", KEYS[2])
end
return 1
`)
	chatRateSeq int64
)

// PairReservation 表示一次发送额度预占。
// 上层在消息真正落库失败时调用 Release 回滚本次占用。
type PairReservation struct {
	windowKey string
	dailyKey  string
	member    string
}

// Release 撤销一次预占（消息未落库、或命中幂等键时）。
func (r *PairReservation) Release(deps redis_service.Options) error {
	client := deps.Client
	if r == nil || client == nil {
		return nil
	}
	_, err := chatPairReleaseScript.Run(context.Background(), client,
		[]string{r.windowKey, r.dailyKey}, r.member).Result()
	return err
}

// ReserveChatPairRate 尝试为 (sender, receiver) 预占一次发送额度。
//
// windowSec / dailyMax 由调用方从运行时配置现读（后台可改），非法值由调用方
// 先用 siteconf.NormalizeChatRate 收敛；本函数只再兜一层，避免误传 0 导致
// "窗口 0 秒 = 不限流 / 日额度 0 = 全站禁言"这类静默失效。
//
// 返回值：reservation 非空=成功；为空且 limitedBy 非空=被限流；err 非空=Redis 异常。
// Redis 未初始化时返回 error（失败关闭，不放行）。
func ReserveChatPairRate(deps redis_service.Options, senderID, receiverID ctype.ID, now time.Time, windowSec, dailyMax int) (*PairReservation, string, error) {
	if windowSec <= 0 {
		windowSec = 10
	}
	if dailyMax <= 0 {
		dailyMax = 99
	}
	window := time.Duration(windowSec) * time.Second
	// 窗口 key 的 TTL 取 2 倍窗口，留一点冗余避免频繁重建；最少 20 秒。
	windowTTL := window * 2
	if windowTTL < 20*time.Second {
		windowTTL = 20 * time.Second
	}
	client := deps.Client
	if client == nil {
		return nil, "", fmt.Errorf("redis 未初始化")
	}
	member := buildChatRateMember(senderID, now)
	windowKey := chatPairWindowKey(senderID, receiverID)
	dailyKey := chatPairDailyKey(senderID, receiverID, now)

	result, err := chatPairReserveScript.Run(context.Background(), client,
		[]string{windowKey},
		now.UnixMilli(),
		window.Milliseconds(),
		chatPairWindowLimit,
		dailyMax,
		dailyKey,
		member,
		windowTTL.Milliseconds(),
		chatPairDailyTTL.Milliseconds(),
	).Result()
	if err != nil {
		return nil, "", err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return nil, "", fmt.Errorf("聊天限流返回结果异常: %v", result)
	}
	allowed, ok := values[0].(int64)
	if !ok {
		return nil, "", fmt.Errorf("聊天限流状态异常: %v", values[0])
	}
	reason, ok := values[1].(string)
	if !ok {
		return nil, "", fmt.Errorf("聊天限流原因异常: %v", values[1])
	}
	if allowed == 0 {
		return nil, reason, nil
	}
	return &PairReservation{windowKey: windowKey, dailyKey: dailyKey, member: member}, "", nil
}

func buildChatRateMember(senderID ctype.ID, now time.Time) string {
	seq := atomic.AddInt64(&chatRateSeq, 1)
	return senderID.String() + ":" + strconv.FormatInt(now.UnixNano(), 10) + ":" + strconv.FormatInt(seq, 10)
}

func chatPairWindowKey(senderID, receiverID ctype.ID) string {
	return "chat:rate:window:" + senderID.String() + ":" + receiverID.String()
}

// chatPairDailyKey 按自然日（本地时区）分桶，避免跨日 key 混淆。
func chatPairDailyKey(senderID, receiverID ctype.ID, now time.Time) string {
	return "chat:rate:daily:" + now.Format("20060102") + ":" + senderID.String() + ":" + receiverID.String()
}
