package middleware

import (
	"context"

	"myblogx/utils/jwts"
)

// TokenVerifier 校验访问令牌并返回声明。
//
// 为什么是窄接口，而不是直接收 user_service.Authenticator：
//
//	middleware 只该关心"这张 token 有效吗、持有人是谁"，
//	不该知道用户表长什么样、会话表怎么查、黑名单存在哪。
//
// 更重要的是认证只能有一份实现。S4.1 把认证迁进 internal/auth 之后，
// 路由用的是新实现，而这里的中间件还挂着旧的 user_service.Authenticator ——
// 两套逻辑并存：新实现会查黑名单与令牌版本，旧的不一定。
// 那种"登出了但 token 还能用"的问题就出在这种并存里。
//
// 实现由装配层提供（见 app.WireHTTP 的 authVerifier），middleware 不依赖 auth 包。
type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, raw string) (*jwts.MyClaims, error)
}
