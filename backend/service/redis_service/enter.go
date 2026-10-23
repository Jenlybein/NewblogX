package redis_service

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Deps struct {
	Client *redis.Client
	Logger *logrus.Logger
}

func NewDeps(client *redis.Client, logger *logrus.Logger) Deps {
	return Deps{
		Client: client,
		Logger: logger,
	}
}

var (
	// 锁释放脚本：仅当锁值与 token 一致时才释放锁，避免误删别的实例的锁。
	// Lua 脚本的本质是把 “查 token + 删锁” 打包成一个 Redis 原子命令，保证执行不会被任何并发操作打断。
	releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)
)

// lockArticleSync 尝试加锁，成功返回解锁函数；若锁已被占用返回 nil,nil。
func LockArticleSync(deps Deps, ctx context.Context, lockKey string, lockTTL time.Duration) (func(), error) {
	if deps.Client == nil {
		return nil, redis.Nil
	}
	// 生成锁 token，作为锁拥有者标识。
	token := strconv.FormatInt(time.Now().UnixNano(), 10)

	// SETNX + TTL：抢锁并设置超时。
	locked, err := deps.Client.SetNX(ctx, lockKey, token, lockTTL).Result()
	if err != nil {
		return nil, err
	}

	// 未抢到锁，返回 nil 让上层跳过。
	if !locked {
		return nil, nil
	}

	// 返回解锁闭包：仅在 token 匹配时删除锁。
	return func() {
		// KEYS[1] = lockKey Redis Key）；
		// ARGV[1] = token（抢锁时生成的唯一标识）。
		if _, err := releaseLockScript.Run(ctx, deps.Client, []string{lockKey}, token).Result(); err != nil && deps.Logger != nil {
			deps.Logger.Errorf("同步文章任务释放锁失败: 错误=%v", err)
		}
	}, nil
}
