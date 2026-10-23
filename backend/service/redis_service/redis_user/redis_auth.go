// 提供基于 Redis 的认证安全控制功能：登录失败次数限制与锁定、邮箱发送频率限制
package redis_user

import (
	"context"
	"time"

	siteconf "myblogx/conf/site"
	"myblogx/service/redis_service"
)

// CheckLoginAllowed 检查账号和IP是否允许登录
func CheckLoginAllowed(deps redis_service.Deps, account, ip string) bool {
	if deps.Client == nil {
		return true
	}
	ctx := context.Background()

	// 检查账号和IP是否都未被锁定
	return !keyExists(deps, ctx, loginLockUserKey(account)) && !keyExists(deps, ctx, loginLockIPKey(ip))
}

// RecordLoginFailure 记录登录失败，并在达到阈值时触发锁定
func RecordLoginFailure(deps redis_service.Deps, loginConf siteconf.Login, account, ip string) {
	if deps.Client == nil {
		return
	}
	ctx := context.Background()
	window := time.Duration(loginConf.LoginFailWindowMinute) * time.Minute
	// 分别记录账号和IP的失败次数
	recordFailure(deps, ctx, loginFailUserKey(account), loginLockUserKey(account), window, loginConf.LoginFailUserMax)
	recordFailure(deps, ctx, loginFailIPKey(ip), loginLockIPKey(ip), window, loginConf.LoginFailIPMax)
}

// ResetLoginFailures 重置账号和IP的登录失败计数与锁定状态
func ResetLoginFailures(deps redis_service.Deps, account, ip string) {
	if deps.Client == nil {
		return
	}
	ctx := context.Background()
	// 删除账号和IP的失败计数key、锁定key
	_ = deps.Client.Del(ctx,
		loginFailUserKey(account),
		loginFailIPKey(ip),
		loginLockUserKey(account),
		loginLockIPKey(ip),
	).Err()
}

// AllowEmailSend 检查邮箱发送是否在频率限制范围内
func AllowEmailSend(deps redis_service.Deps, loginConf siteconf.Login, email, ip string, sendType int8) bool {
	if deps.Client == nil {
		return true
	}
	ctx := context.Background()
	window := time.Duration(loginConf.EmailSendWindowSecond) * time.Second
	// 分别检查邮箱和IP的发送频率
	okEmail := allowWithinWindow(deps, ctx, emailSendKeyByEmail(email, sendType), window, loginConf.EmailSendPerEmailMax)
	okIP := allowWithinWindow(deps, ctx, emailSendKeyByIP(ip, sendType), window, loginConf.EmailSendPerIPMax)
	// 只有邮箱和IP都未超限才允许发送
	return okEmail && okIP
}

// recordFailure 记录失败次数，并在达到阈值时设置锁定
func recordFailure(deps redis_service.Deps, ctx context.Context, countKey, lockKey string, ttl time.Duration, max int64) {
	// countKey - 失败计数的Redis Key
	//	lockKey - 锁定状态的Redis Key
	//	ttl - 时间窗口（计数和锁定的过期时间）
	//	max - 最大失败次数阈值

	// 增加失败计数（首次调用会自动创建Key并初始化为1）
	count, err := deps.Client.Incr(ctx, countKey).Result()
	if err != nil {
		return
	}
	// 如果是第一次计数，设置Key的过期时间
	if count == 1 {
		_ = deps.Client.Expire(ctx, countKey, ttl).Err()
	}
	// 如果达到最大失败次数，设置锁定Key
	if count >= max {
		_ = deps.Client.Set(ctx, lockKey, "1", ttl).Err()
	}
}

// allowWithinWindow 检查操作是否在时间窗口内的允许次数范围内
func allowWithinWindow(deps redis_service.Deps, ctx context.Context, key string, ttl time.Duration, max int64) bool {
	//	key - 计数的Redis Key
	//	ttl - 时间窗口
	//	max - 最大允许次数

	// 增加操作计数（首次调用会自动创建Key并初始化为1）
	count, err := deps.Client.Incr(ctx, key).Result()
	if err != nil {
		// Redis操作失败时默认允许（降级处理）
		return true
	}
	// 如果是第一次计数，设置Key的过期时间
	if count == 1 {
		_ = deps.Client.Expire(ctx, key, ttl).Err()
	}
	// 检查计数是否在允许范围内
	return count <= max
}
