package app

import (
	"context"

	"myblogx/internal/apperr"
	authdomain "myblogx/internal/auth/domain"
	"myblogx/internal/kernel/enum"
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
//
// 角色不在 {管理员, 注册用户} 内时返回"请先登录"（ADR-0017 决策 3）：
// 受限访客（guest / role=3）与库里未入枚举的 role=0 在这里就按未登录取，
// 于是 AuthMiddleware 拒绝、OptionalAuthMiddleware 按匿名继续。
func (authVerifier) VerifyAccessToken(ctx context.Context, raw string) (*jwts.MyClaims, error) {
	if authService == nil {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	p, err := authService.Authenticate(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err := requireWritablePrincipal(p); err != nil {
		return nil, err
	}
	return &jwts.MyClaims{Claims: jwts.Claims{
		UserID:    p.UserID,
		SessionID: p.SessionID,
		Role:      enum.RoleType(p.Role),
		Username:  p.Username,
	}}, nil
}

// requireWritablePrincipal 是「受限访客视同未登录」的唯一判定点（ADR-0017 决策 3）。
//
// 只有管理员（1）与注册用户（2）算登录；guest（3）与 role=0（库里的列默认值，
// 不在枚举里）都按未登录处理：能浏览公开内容，不能创建/发布/互动，
// 也看不到自己的历史内容。
//
// 判定刻意放在 principal → 身份的这一次转换里，而不是各模块的 Viewer 里：
// 其中多个模块把调用者压成 uint64 / bool，结构上表达不了"访客"，
// 逐个改只会留下漏口。同包另外两处直接调 Authenticate 的重建点
// （搜索的 tokenVerifier、私信的 chatAuthBridge）也复用这个函数
// —— 它们不走中间件，只改 VerifyAccessToken 盖不住（私信 WS 直接读 header 令牌）。
//
// 为什么返回错误而不是把角色清空：下游假定 claims 存在，清空角色只会让它们
// 把访客当成普通用户——那正是旧行为。返回错误则让调用方按"没有身份"处理。
func requireWritablePrincipal(p *authdomain.Principal) error {
	if p == nil {
		return apperr.New(apperr.CodeAuthRequired)
	}
	switch enum.RoleType(p.Role) {
	case enum.RoleAdmin, enum.RoleUser:
		return nil
	default:
		return apperr.New(apperr.CodeAuthRequired)
	}
}
