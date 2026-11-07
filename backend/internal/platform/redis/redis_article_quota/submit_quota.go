// Package redis_article_quota 维护「投稿配额」的当日计数（票 304 / ADR-0030）。
//
// 配额口径（spec #302 决策 18–20）：
//   - 计量单位是**提交动作**：被打回也计；因系统不可用而失败的提交由调用方
//     释放预占，因此不占额度；
//   - 每人每自然日一个默认值（后台可调），管理员豁免由用例层判断，本包不认识角色；
//   - 按日期分桶的计数键 + 预占 / 释放，跨零点靠键的 TTL 自然过期，不依赖定时任务。
//
// 为什么形状照抄私信限流（internal/platform/redis/redis_chat/risk_limit.go）：
// 两处解决的是同一类问题——"在 Redis 里原子的占一个当日额度，失败要能还回去"。
// 同一个 idiom 用两遍比发明第二种更省维护：读者只需要懂一遍 Lua 脚本的意图。
//
// 本包只做计数，不读配置、不认识管理员、不 import 任何业务模块：
// 额度上限由调用方每次现读后传入（后台改完下一次请求即生效）；
// 错误码翻译与角色豁免在 article 模块的适配器与用例层。
package redis_article_quota

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	redis_service "myblogx/internal/platform/redis"
)

// submitQuotaDailyTTL 是当日计数键的存活时间。
//
// 取 26 小时而不是"到今晚零点"：键里已经带了日期，跨零点时下一个自然日用的是
// 另一个键，旧键只需要自己过期即可。多留 2 小时是给时区设置与手工排查留余量，
// 与私信日额度键同一条口径（ADR-0026）。
const submitQuotaDailyTTL = 26 * time.Hour

var (
	// submitQuotaReserveScript 原子地完成"读计数 → 判额度 → 自增 + 设 TTL"。
	//
	// 三步必须在一次 Lua 调用里完成：分开做会出现两个并发提交都读到 14、
	// 于是都放行、当日实际用掉 16 次的竞态。
	//
	// 返回 1 = 预占成功；0 = 额度已用完（这是正常结论，不是 Redis 故障）。
	submitQuotaReserveScript = redis.NewScript(`
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
	// submitQuotaReleaseScript 归还一次预占。
	//
	// `used > 0` 的判断必不可少：键可能已经自然过期（跨了零点），
	// 此时 DECR 会写出 -1，把第二天的额度凭空放大一次。
	submitQuotaReleaseScript = redis.NewScript(`
local dailyKey = KEYS[1]
local used = tonumber(redis.call("GET", dailyKey) or "0")
if used > 0 then
	redis.call("DECR", dailyKey)
end
return 1
`)
)

// Reservation 表示一次已占用的当日投稿额度。
//
// 上层在文章没有落库（任何失败）时调用 Release 归还本次占用。
type Reservation struct {
	key string
	// released 保证同一份预占最多归还一次：调用点常写成"显式 Release + defer"，重复
	// 归还会把当日额度放大，等于凭空多出投稿次数。
	released atomic.Bool
}

// Release 归还本次预占。重复调用是安全的（幂等、不报错、不重复扣减）。
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
	_, err := submitQuotaReleaseScript.Run(context.Background(), client, []string{r.key}).Result()
	return err
}

// ReserveSubmitQuota 尝试为 userID 预占一次当日投稿额度。
//
// dailyMax 由调用方从生效值模块的现读闭包取（后台可改、已归一化）；本函数只再
// 兜一层，避免误传 0 或负数被当成"全站禁止投稿"。
//
// 返回值：
//   - (reservation, true, nil)：预占成功，调用方在提交失败时必须 Release；
//   - (nil, false, nil)：当日额度已用完（正常业务结论，不是故障）；
//   - (nil, false, err)：Redis 异常——失败关闭，调用方不得放行提交。
func ReserveSubmitQuota(deps redis_service.Options, userID ctype.ID, now time.Time, dailyMax int) (*Reservation, bool, error) {
	if dailyMax <= 0 {
		dailyMax = config.DefaultArticleSubmitDailyMax
	}
	client := deps.Client
	if client == nil {
		return nil, false, fmt.Errorf("redis 未初始化")
	}
	key := submitQuotaDailyKey(userID, now)

	result, err := submitQuotaReserveScript.Run(context.Background(), client,
		[]string{key},
		dailyMax,
		submitQuotaDailyTTL.Milliseconds(),
	).Result()
	if err != nil {
		return nil, false, err
	}
	allowed, ok := result.(int64)
	if !ok {
		return nil, false, fmt.Errorf("投稿配额返回结果异常: %v", result)
	}
	if allowed == 0 {
		return nil, false, nil
	}
	return &Reservation{key: key}, true, nil
}

// submitQuotaDailyKey 按自然日（本地时区）分桶，避免跨日计数混在一起。
func submitQuotaDailyKey(userID ctype.ID, now time.Time) string {
	return "article:submit:daily:" + now.Format("20060102") + ":" + userID.String()
}
