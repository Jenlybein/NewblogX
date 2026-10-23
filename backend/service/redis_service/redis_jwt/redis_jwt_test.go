package redis_jwt_test

import (
	"myblogx/conf"
	"myblogx/models/enum"
	"myblogx/service/redis_service"
	redis_jwt "myblogx/service/redis_service/redis_jwt"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"testing"
)

func TestBlackTypeHelpers(t *testing.T) {
	if redis_jwt.UserBlackType.String() == "" {
		t.Fatal("BlackType.String 不应为空")
	}
	if redis_jwt.UserBlackType.RedisValue() != "1" {
		t.Fatalf("BlackType.RedisValue 异常: %s", redis_jwt.UserBlackType.RedisValue())
	}

	bt, err := redis_jwt.BlackTypeFromRedisValue("2")
	if err != nil || bt != redis_jwt.AdminBlackType {
		t.Fatalf("BlackTypeFromRedisValue 失败: bt=%v err=%v", bt, err)
	}
	if _, err = redis_jwt.BlackTypeFromRedisValue("x"); err == nil {
		t.Fatal("非法字符串应报错")
	}
	if redis_jwt.DeviceBlackType.String() != "其他设备已登录" {
		t.Fatalf("BlackType.String 文案异常: %s", redis_jwt.DeviceBlackType.String())
	}
}

func TestTokenBlacklistFlow(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "test-secret",
			Issuer: "test",
		},
	})
	jwtConf := testutil.Config().Jwt
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}

	token, err := jwts.GetToken(jwtConf, jwts.Claims{
		UserID:   1,
		Role:     enum.RoleUser,
		Username: "u1",
	})
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	redis_jwt.SetTokenBlack(deps, jwtConf, token, redis_jwt.UserBlackType)
	blackType, ok := redis_jwt.HasTokenBlack(deps, token)
	if ok {
		t.Fatalf("黑名单 token 应返回 ok=false: %s", blackType.String())
	}
	if blackType != redis_jwt.UserBlackType {
		t.Fatalf("黑名单类型错误: got=%v want=%v", blackType, redis_jwt.UserBlackType)
	}

	// 黑名单不存在时应允许通过
	blackType, ok = redis_jwt.HasTokenBlack(deps, "not-exist-token")
	if !ok {
		t.Fatal("不存在黑名单的 token 应返回 ok=true")
	}
	if blackType != 0 {
		t.Fatalf("未命中黑名单时类型应为 0, got=%v", blackType)
	}
}
