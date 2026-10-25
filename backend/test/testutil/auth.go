package testutil

import (
	"context"

	"myblogx/conf"
	"myblogx/internal/apperr"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_jwt"
	"myblogx/utils/jwts"
)

// AuthVerifier 实现 middleware.TokenVerifier，供路由/中间件测试使用。
//
// 只验签名与有效期，不查会话表、不查黑名单、不查用户状态 ——
// 路由测试关心的是"这条请求走到哪个业务分支"，鉴权链路的完整性由
// internal/auth 自己的测试覆盖。用真实 auth 服务会让每个路由测试
// 都要准备会话与用户行，收益不抵成本。
//
// 与 IssueAccessToken 配对使用：那边用 testutil.Config().Jwt 签发，
// 这里用同一份配置解析。
type AuthVerifier struct {
	Config conf.Jwt

	// Redis 非空时检查令牌黑名单。
	//
	// 中间件测试必须带上它：TestAuthAndAdminMiddleware 明确断言
	// "登出后的 token 不能再用"。如果 verifier 只验签名，那条断言会红 ——
	// 而那不是测试的问题，是简化过度。
	Redis redis_service.Options
}

// VerifyAccessToken 实现 middleware.TokenVerifier。
func (v AuthVerifier) VerifyAccessToken(_ context.Context, raw string) (*jwts.MyClaims, error) {
	if raw == "" {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	// 注意 HasTokenBlack 的返回语义是**反直觉**的：
	// 第二个返回值 ok=true 表示"不在黑名单、可以放行"；命中黑名单返回 false。
	// Redis 不可用或读取出错时也返回 false（fail-closed）。
	// 写成 if black { 拒绝 } 会把所有正常 token 拒掉 —— 这里踩过一次。
	if _, allowed := redis_jwt.HasTokenBlack(v.Redis, raw); !allowed {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	return jwts.ParseToken(v.Config, raw)
}
