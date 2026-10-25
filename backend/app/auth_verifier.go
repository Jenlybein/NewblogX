package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/jwts"
)

// authVerifier 用 internal/auth 校验访问令牌。
//
// 这是全站唯一的认证实现：路由里的 authHandler 与 middleware.AuthMiddleware
// 都走这里。迁移中期曾出现"路由用新实现、中间件还挂旧 Authenticator"的并存状态，
// 那类"登出了但 token 还能用"的问题就藏在并存里；旧实现已随 user_service 一起删除。
//
// 用延迟绑定而不是构造顺序：authService 在 WireUser 里赋值，而 middleware
// 的 RuntimeOptions 在它之前构造。router.Run 在两者之后调用，所以运行时一定有值。
type authVerifier struct{}

// VerifyAccessToken 实现 middleware.TokenVerifier。
//
// 把 Principal 映射成 jwts.MyClaims 而不是改下游：十几个模块都用
// jwts.GetClaimsByGin 读 UserID / Role。TokenVersion 不回填，
// 因为版本校验已经在 Authenticate 内部完成，之后再读它只会误用。
func (authVerifier) VerifyAccessToken(ctx context.Context, raw string) (*jwts.MyClaims, error) {
	if authService == nil {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	p, err := authService.Authenticate(ctx, raw)
	if err != nil {
		return nil, err
	}
	return &jwts.MyClaims{Claims: jwts.Claims{
		UserID:    ctype.ID(p.UserID),
		SessionID: ctype.ID(p.SessionID),
		Role:      enum.RoleType(p.Role),
		Username:  p.Username,
	}}, nil
}
