package redis_user

import (
	"context"
	"fmt"
	"myblogx/service/redis_service"
	"strconv"
)

const (
	usernameSeqKey     = "seq:user:username"
	usernameSeqStartAt = 100000
)

// NextAutoUsername 从 Redis 中申请下一个系统用户名。
// 约定初始化值为 100000，因此首个自动发放的用户名是 100001。
func NextAutoUsername(deps redis_service.Deps) (string, error) {
	if deps.Client == nil {
		return "", fmt.Errorf("redis 未初始化，无法生成用户名")
	}

	ctx := context.Background()

	if err := deps.Client.SetNX(ctx, usernameSeqKey, usernameSeqStartAt, 0).Err(); err != nil {
		return "", fmt.Errorf("初始化用户名序列失败: %w", err)
	}

	seq, err := deps.Client.Incr(ctx, usernameSeqKey).Result()
	if err != nil {
		return "", fmt.Errorf("生成用户名失败: %w", err)
	}

	return strconv.FormatInt(seq, 10), nil
}
