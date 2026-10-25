// Package rediscounter 把 Redis hash 里的计数增量刷回数据库。
//
// 背景：浏览、点赞、收藏、评论这些计数先累加在 Redis（抗住热点写入），
// 再由定时任务按批落库。落库失败必须把增量退回 Redis，否则这批数据就丢了。
//
// 本包只负责**刷盘机制**：双缓冲搬运、解析、逐条落库、失败回补。
// "哪个 key 对应哪张表的哪一列"是各业务模块的知识，通过 Task.ApplyDelta 注入。
//
// 双缓冲的作用：刷盘期间业务还在往 active key 累加。直接读 active key 会读到
// 一半被清空（丢失刷盘期间的新增量）。所以先用 RENAME 把 active 换成 sync key，
// 业务随后自动写到新的 active key 上，两边互不干扰。
package rediscounter

import (
	"context"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// Logger 是窄日志接口，避免本包依赖具体的日志实现。
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Task 描述一个计数刷盘任务。
type Task struct {
	// Name 是任务名，只用于日志。
	Name string
	// Metric 是指标名（如"点赞数"），只用于日志；为空时不打印。
	Metric string
	// ActiveKey 是业务写入的 hash key。
	ActiveKey string
	// SyncKey 是刷盘期间的临时 key，必须与 ActiveKey 不同且稳定。
	SyncKey string
	// IDName 是被计数对象的名称（如 article_id），只用于日志。
	IDName string
	// ApplyDelta 把单条增量写进库。
	ApplyDelta func(id uint64, delta int) error
}

// 双缓冲搬运脚本。返回值语义：
//
//	0 —— 没有活可干（active key 不存在），跳过本轮；
//	1 —— 可以刷（刚 RENAME 成功，或 sync key 里还有上一轮的残留）。
//
// **sync key 已存在时故意继续**：那是上一轮在 RENAME 之后、写完库之前崩掉的残留。
// 若这时直接跳过，这批增量就永久卡在 sync key 里再没人处理。
// 继续处理的正确性依赖外层分布式锁 —— 同一时刻只有一个实例在跑；
// 没有锁时两个实例会同时处理同一个 sync key，把同一笔增量加两遍。
var prepareScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
	return 1
end
if redis.call("EXISTS", KEYS[1]) == 0 then
	return 0
end
redis.call("RENAME", KEYS[1], KEYS[2])
return 1
`)

// Flusher 执行刷盘。
type Flusher struct {
	client *redis.Client
	logger Logger
}

// New 构造刷盘器。client 为 nil 时 Flush 直接返回 0（无缓存则无增量）。
func New(client *redis.Client, logger Logger) *Flusher {
	return &Flusher{client: client, logger: logger}
}

// Flush 把 Task 对应 key 的增量刷进库，返回处理的条目数。
//
// 单条落库失败不会中断整批：把该条的增量加回 active key 等下轮重试，
// 其余条目继续写。这样一条坏数据不会卡住所有人的计数。
func (f *Flusher) Flush(ctx context.Context, t Task) (int, error) {
	if f == nil || f.client == nil || t.ApplyDelta == nil {
		return 0, nil
	}
	prefix := t.Name
	if t.Metric != "" {
		prefix += "同步" + t.Metric
	}

	ret, err := prepareScript.Run(ctx, f.client, []string{t.ActiveKey, t.SyncKey}).Int()
	if err != nil {
		return 0, err
	}
	if ret != 1 {
		return 0, nil
	}

	raw, err := f.client.HGetAll(ctx, t.SyncKey).Result()
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, f.drop(ctx, t.SyncKey)
	}

	deltas := make(map[uint64]int, len(raw))
	for idStr, deltaStr := range raw {
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			f.warnf("%s忽略非法%s: Redis键=%s %s=%s", prefix, t.IDName, t.ActiveKey, t.IDName, idStr)
			continue
		}
		delta, err := strconv.Atoi(deltaStr)
		if err != nil {
			f.warnf("%s忽略非法增量: Redis键=%s %s=%s 增量=%s", prefix, t.ActiveKey, t.IDName, idStr, deltaStr)
			continue
		}
		if delta == 0 {
			continue
		}
		deltas[id] += delta
	}

	if len(deltas) == 0 {
		return 0, f.drop(ctx, t.SyncKey)
	}

	for id, delta := range deltas {
		if err := t.ApplyDelta(id, delta); err != nil {
			f.warnf("%s写库失败，回补缓存: Redis键=%s %s=%d 增量=%d 错误=%v",
				prefix, t.ActiveKey, t.IDName, id, delta, err)
			// 回补到 active key：下轮会再取到。这里失败只能记日志 ——
			// 已经没别的补偿手段，至少要让它在日志里留痕。
			if requeueErr := f.client.HIncrBy(ctx, t.ActiveKey,
				strconv.FormatUint(id, 10), int64(delta)).Err(); requeueErr != nil {
				f.errorf("%s回补缓存失败: Redis键=%s %s=%d 增量=%d 错误=%v",
					prefix, t.ActiveKey, t.IDName, id, delta, requeueErr)
			}
		}
	}

	if err := f.drop(ctx, t.SyncKey); err != nil {
		return 0, err
	}
	return len(deltas), nil
}

// drop 删除 sync key，表示本轮结束。
func (f *Flusher) drop(ctx context.Context, key string) error {
	return f.client.Del(ctx, key).Err()
}

func (f *Flusher) warnf(format string, args ...any) {
	if f.logger != nil {
		f.logger.Warnf(format, args...)
	}
}

func (f *Flusher) errorf(format string, args ...any) {
	if f.logger != nil {
		f.logger.Errorf(format, args...)
	}
}
