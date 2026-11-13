// Package redis_ai_quota 维护「AI 每日额度」的当日计数（ADR-0028）。
//
// 口径与投稿配额（redis_article_quota）同一套 idiom：
//   - 计量单位由调用方决定（出站调用一次算一次；评分按动作算一次）；
//   - 每人每自然日、每场景一档；场景名由调用方传入，本包不认识业务场景；
//   - 按日期分桶的计数键 + 预占 / 释放，跨零点靠键的 TTL 自然过期；
//   - 上限为 0 表示不限，本包直接放行且不写键（省一次写）。
//
// 为什么形状照抄而不是新发明：这是第三处"在 Redis 里原子地占一个当日额度、
// 失败要能还回去"的地方（前三处是私信、投稿）。同一个 Lua 意图用三遍，
// 读者只需要懂一遍。
package redis_ai_quota

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
)

// dailyTTL 是当日计数键的存活时间。
//
// 取 26 小时而不是"到今晚零点"：键里已经带了日期，跨零点用的是另一个键，
// 旧键只需自己过期。多留 2 小时给时区与手工排查留余量（与 ADR-0018/ADR-0022
// 同一条口径）。
const dailyTTL = 26 * time.Hour

var (
	// reserveScript 原子地完成"读计数 → 判额度 → 自增 + 设 TTL"。
	//
	// 三步必须在一次 Lua 调用里完成，否则两个并发调用会同时读到上限 - 1、
	// 都放行，当日实际多扣一次。
	// 返回 1 = 成功；0 = 额度已用完（正常结论，不是故障）。
	reserveScript = redis.NewScript(`
local dailyKey = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

local used = tonumber(redis.call("GET", dailyKey) or "0")
if used >= limit then
	return 0
end
redis.call("INCR", dailyKey)
redis.call("PEXPIRE", dailyKey, ttl)
return 1
`)
	// releaseScript 归还一次预占。
	//
	// `used > 0` 必不可少：键可能已经自然过期（跨了零点），
	// 此时 DECR 会写出 -1，把第二天的额度凭空放大一次。
	releaseScript = redis.NewScript(`
local dailyKey = KEYS[1]
local used = tonumber(redis.call("GET", dailyKey) or "0")
if used > 0 then
	redis.call("DECR", dailyKey)
end
return 1
`)
)

// Reservation 表示一次已占用的当日额度。
type Reservation struct {
	key string
	// released 保证同一份预占最多归还一次，否则一次失败会放大当日额度。
	released atomic.Bool
}

// Release 归还本次预占（幂等：重复调用不重复扣减、不报错）。
func (r *Reservation) Release(deps redis_service.Options) error {
	if r == nil {
		return nil
	}
	client := deps.Client
	if client == nil {
		return nil
	}
	if r.released.Swap(true) {
		return nil
	}
	_, err := releaseScript.Run(context.Background(), client, []string{r.key}).Result()
	return err
}

// ReserveAIDailyQuota 尝试为 (scene, userID) 预占一次当日额度。
//
// 返回值：
//   - (reservation, true, nil)：预占成功，调用方在失败时 Release；
//   - (nil, true, nil)：该场景不限（limit <= 0），没有写键，也不需要归还；
//   - (nil, false, nil)：当日额度已用完（正常业务结论）；
//   - (nil, false, err)：Redis 异常 —— 调用方必须失败关闭。
func ReserveAIDailyQuota(deps redis_service.Options, scene string, userID ctype.ID, now time.Time, limit int) (*Reservation, bool, error) {
	if limit <= 0 {
		return nil, true, nil
	}
	client := deps.Client
	if client == nil {
		return nil, false, fmt.Errorf("redis 未初始化")
	}
	key := dailyKey(scene, userID, now)

	result, err := reserveScript.Run(context.Background(), client,
		[]string{key},
		limit,
		dailyTTL.Milliseconds(),
	).Result()
	if err != nil {
		return nil, false, err
	}
	allowed, ok := result.(int64)
	if !ok {
		return nil, false, fmt.Errorf("AI 额度返回结果异常: %v", result)
	}
	if allowed == 0 {
		return nil, false, nil
	}
	return &Reservation{key: key}, true, nil
}

// dailyKey 按 场景 + 自然日（本地时区）+ 用户 分桶。
//
// 场景在最前：排查"某个场景今天全站用了多少"时前缀能直接裁剪。
func dailyKey(scene string, userID ctype.ID, now time.Time) string {
	return "ai:daily:" + scene + ":" + now.Format("20060102") + ":" + userID.String()
}
