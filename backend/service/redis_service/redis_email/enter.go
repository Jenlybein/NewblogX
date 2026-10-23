package redis_email

import (
	"context"
	"fmt"
	"myblogx/service/redis_service"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const emailVerifyKeyPrefix = "email_verify:"

var verifyScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
	return {0, ""}
end

local realCode = redis.call("HGET", KEYS[1], "code")
if not realCode then
	return {0, ""}
end

if realCode ~= ARGV[1] then
	local failCount = redis.call("HINCRBY", KEYS[1], "fail_count", 1)
	local maxFail = tonumber(redis.call("HGET", KEYS[1], "max_fail") or "0")
	if failCount >= maxFail then
		redis.call("DEL", KEYS[1])
	end
	return {-1, ""}
end

local email = redis.call("HGET", KEYS[1], "email")
redis.call("DEL", KEYS[1])
return {1, email or ""}
`)

// Store 保存邮箱验证码，timeoutMinute 为过期分钟，maxFailCount 为最大失败次数。
func Store(deps redis_service.Deps, id, email, code string, timeoutMinute, maxFailCount int) error {
	if deps.Client == nil {
		return fmt.Errorf("redis 未初始化")
	}

	key := emailVerifyKey(id)
	ctx := context.Background()
	pipe := deps.Client.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"email":      email,
		"code":       code,
		"fail_count": 0,
		"max_fail":   maxFailCount,
	})
	pipe.Expire(ctx, key, time.Duration(timeoutMinute)*time.Minute)

	if _, err := pipe.Exec(ctx); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("邮件验证码存储失败: %v", err)
		}
		return err
	}
	return nil
}

func Delete(deps redis_service.Deps, id string) error {
	if deps.Client == nil {
		return fmt.Errorf("redis 未初始化")
	}
	if err := deps.Client.Del(context.Background(), emailVerifyKey(id)).Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("邮件验证码删除失败: %v", err)
		}
		return err
	}
	return nil
}

// Verify 校验验证码，成功后返回邮箱并删除该验证码记录（一次性消费）。
func Verify(deps redis_service.Deps, id, code string) (email string, ok bool, err error) {
	if deps.Client == nil {
		return "", false, fmt.Errorf("redis 未初始化")
	}

	res, err := verifyScript.Run(
		context.Background(),
		deps.Client,
		[]string{emailVerifyKey(id)},
		code,
	).Result()
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("邮件验证码校验失败: %v", err)
		}
		return "", false, err
	}

	items, ok := res.([]any)
	if !ok || len(items) != 2 {
		return "", false, fmt.Errorf("邮件验证码校验返回结果异常: %v", res)
	}

	status, ok := toInt64(items[0])
	if !ok {
		return "", false, fmt.Errorf("邮件验证码状态解析异常: %v", items[0])
	}
	if status != 1 {
		return "", false, nil
	}

	return toString(items[1]), true, nil
}

func emailVerifyKey(id string) string {
	return fmt.Sprintf("%s%s", emailVerifyKeyPrefix, id)
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		return int64(n), true
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case []byte:
		parsed, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}
