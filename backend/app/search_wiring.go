package app

import (
	"context"

	authapp "myblogx/internal/auth/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/http/router"
	redis_service "myblogx/internal/platform/redis"
	searchhttp "myblogx/internal/search/adapter/http"
	searchmysql "myblogx/internal/search/adapter/mysql"
	searchredis "myblogx/internal/search/adapter/redis"
	"myblogx/internal/search/domain"
)

// WireSearch 装配搜索模块并注入路由处理器。
func WireSearch(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	h := searchhttp.NewHandler(
		searchredis.NewCounters(redisOptions),
		searchmysql.NewPrefs(infra.DB),
		infra.ESClient,
		infra.Config.ES.Index,
		tokenVerifier{svc: authService},
	)
	router.SetSearchHandler(h)
}

// tokenVerifier 用认证模块校验访问令牌。
type tokenVerifier struct {
	svc *authapp.Service
}

// Verify 实现 searchhttp.Verifier。
func (v tokenVerifier) Verify(ctx context.Context, token string) (domain.Viewer, bool) {
	if v.svc == nil {
		return domain.Viewer{}, false
	}
	principal, err := v.svc.Authenticate(ctx, token)
	if err != nil {
		return domain.Viewer{}, false
	}
	return domain.Viewer{
		ID:    ctype.ID(principal.UserID),
		Admin: principal.Role == int8(enum.RoleAdmin),
	}, true
}
