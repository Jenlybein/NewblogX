package redis_chat

import (
	"context"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// chatStrangerWeekQuotaLimit 表示陌生人在自然周内最多允许发送 1 条消息。
	chatStrangerWeekQuotaLimit = 1
	// chatWeekQuotaLimit 表示单向关注关系在自然周内、未被对方回复前最多允许发送 3 条消息。
	chatWeekQuotaLimit = 3
)

var (
	// chatWeekQuotaReserveScript 负责自然周单向配额的预占。
	// 配额用简单计数器表示，key 的过期时间直接设置到本周结束。
	chatWeekQuotaReserveScript = redis.NewScript(`
local limit = tonumber(ARGV[1])
local expireAt = tonumber(ARGV[2])
local current = tonumber(redis.call("GET", KEYS[1]) or "0")

if current >= limit then
	return 0
end

current = redis.call("INCR", KEYS[1])
redis.call("EXPIREAT", KEYS[1], expireAt)
return current
`)
	// chatWeekQuotaReleaseScript 用于消息发送失败时回滚单向自然周配额预占。
	chatWeekQuotaReleaseScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
if current <= 1 then
	redis.call("DEL", KEYS[1])
	return 0
end
return redis.call("DECR", KEYS[1])
`)
)

// WeekQuotaReservation 表示一次自然周单向配额预占。
// 上层在消息最终未发送成功时可以调用 Release 回滚本次占用。
type WeekQuotaReservation struct {
	key string
}

// Release 撤销一次自然周配额预占，用于消息最终未发送成功时回滚。
func (r *WeekQuotaReservation) Release(deps redis_service.Deps) error {
	client := deps.Client
	if r == nil || client == nil {
		return nil
	}

	_, err := chatWeekQuotaReleaseScript.Run(context.Background(), client,
		[]string{r.key},
	).Result()
	return err
}

// ReserveChatWeekQuota 尝试为一条消息预占自然周配额。
// 返回值说明：
// 1. reservation 非空且 allowed=true：预占成功；
// 2. reservation 为空且 allowed=false：本周额度已满；
// 3. err 非空：Redis 执行异常。
func ReserveChatWeekQuota(deps redis_service.Deps, senderID, receiverID ctype.ID, limit int, now time.Time) (*WeekQuotaReservation, bool, error) {
	client := deps.Client
	if client == nil {
		return nil, false, fmt.Errorf("redis 未初始化")
	}

	// 获取当前自然周结束时间
	_, weekEnd := currentWeekRange(now)

	// 获取对应 Key
	key := chatWeekQuotaKey(senderID, receiverID, now)

	// 执行脚本，对应 Key 的数字加 1，过期时间设置到本周结束
	current, err := chatWeekQuotaReserveScript.Run(context.Background(), client,
		[]string{key},
		limit,
		weekEnd.Unix(),
	).Int64()
	if err != nil {
		return nil, false, err
	}
	if current == 0 {
		return nil, false, nil
	}

	return &WeekQuotaReservation{key: key}, true, nil
}

// ResetChatWeekQuota 清空一个方向在当前自然周内的已用额度。
// 这个动作在“对方成功回复后”调用，用于恢复反向的周配额。
func ResetChatWeekQuota(deps redis_service.Deps, senderID, receiverID ctype.ID, now time.Time) error {
	client := deps.Client
	if client == nil {
		return fmt.Errorf("redis 未初始化")
	}
	return client.Del(context.Background(), chatWeekQuotaKey(senderID, receiverID, now)).Err()
}

// chatWeekQuotaKey 返回单向自然周配额 key。
func chatWeekQuotaKey(senderID, receiverID ctype.ID, now time.Time) string {
	weekStart, _ := currentWeekRange(now)
	return "chat:quota:week:" + weekStart.Format("20060102") + ":" +
		senderID.String() + ":" +
		receiverID.String()
}

// currentWeekRange 返回当前时间所在自然周的开始时间和结束时间。
// 这里约定周一 00:00:00 为周起点，结束时间为下周一 00:00:00。
func currentWeekRange(now time.Time) (start, end time.Time) {
	location := now.Location()
	year, month, day := now.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, location)

	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start = midnight.AddDate(0, 0, -(weekday - 1))
	end = start.AddDate(0, 0, 7)
	return start, end
}
