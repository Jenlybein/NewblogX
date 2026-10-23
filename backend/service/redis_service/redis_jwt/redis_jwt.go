package redis_jwt

import (
	"context"
	"fmt"
	"myblogx/conf"
	"myblogx/service/redis_service"
	"myblogx/utils/jwts"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

type BlackType int8

const (
	UserBlackType   BlackType = 1 // 用户注销登录
	AdminBlackType  BlackType = 2 // 管理员注销登录
	DeviceBlackType BlackType = 3 // 其他设备已登录
)

func (b BlackType) String() string {
	switch b {
	case UserBlackType:
		return "用户注销登录"
	case AdminBlackType:
		return "管理员注销登录"
	case DeviceBlackType:
		return "其他设备已登录"
	default:
		return "未知错误"
	}
}

// RedisValue 返回存入 Redis 的枚举值。
func (b BlackType) RedisValue() string {
	return fmt.Sprintf("%d", b)
}

// BlackTypeFromRedisValue 将 Redis 中存储的字符串转换为枚举值。
func BlackTypeFromRedisValue(str string) (BlackType, error) {
	num1, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	return BlackType(num1), nil
}

// SetTokenBlack 将 token 放入 Redis 的黑名单中。
func SetTokenBlack(deps redis_service.Deps, jwtConfig conf.Jwt, token string, blackType BlackType) {
	if deps.Client == nil {
		return
	}

	key := fmt.Sprintf("token_blacklist_%s", token)

	// 获取 token 原本的过期时间
	claims, err := jwts.ParseToken(jwtConfig, token)
	if err != nil || claims == nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("将令牌放入黑名单时解析失败: 错误=%v", err)
		}
		return
	}

	// 计算 token 剩余过期时间
	expire := claims.ExpiresAt - time.Now().Unix()
	if expire <= 0 {
		if deps.Logger != nil {
			deps.Logger.Errorf("令牌已过期，无法放入黑名单")
		}
		return
	}

	_, err = deps.Client.Set(context.Background(), key, blackType.RedisValue(), time.Duration(expire)*time.Second).Result()
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("将令牌放入黑名单时出错: 错误=%v", err)
		}
		return
	}
}

// HasTokenBlack 检查 token 是否在 Redis 的黑名单中。
func HasTokenBlack(deps redis_service.Deps, token string) (blackType BlackType, ok bool) {
	if deps.Client == nil {
		return 0, true
	}

	key := fmt.Sprintf("token_blacklist_%s", token)
	has, err := deps.Client.Get(context.Background(), key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, true
		}
		if deps.Logger != nil {
			deps.Logger.Errorf("检查令牌是否在黑名单时出错: 错误=%v", err)
		}
		return 0, false
	}

	blackType, err = BlackTypeFromRedisValue(has)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("字符串转换为黑名单类型失败: %v", err)
		}
		return 0, false
	}

	return blackType, false
}
