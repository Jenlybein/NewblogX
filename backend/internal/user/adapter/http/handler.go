// Package http 是用户模块的 HTTP 适配器。
//
// 只做三件事：绑定参数、调用用例、把结果/错误翻译成响应。
// 这里不允许出现 SQL（archcheck R1 会拦）。
//
// 本包只保留**路由真实注册**的处理器：公开路径（base/detail/info/admin*）在
// compat.go 里（旧路径、新用例），本文件只放它们共用的装配与助手。
// 曾经这里还有一整套 Update/UpdateSettings/List/Create/ChangeStatus/PublicProfile，
// 迁移到 compat 之后没人调用，但 swag 注解仍然生效——契约因此长出了 6 个
// 运行时 404 的 operation。现在由 router 包的 TestContractMatchesRoutes 兜住这类回潮：
// 注解 != 路由，就必须红。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/platform/http/res"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 是用户模块的 HTTP 处理器。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("user/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// viewer 从 JWT claims 解析操作者。
//
// 没登录时返回匿名（零值）而不是报错：公开主页不需要登录，
// 由用例层按规则决定"匿名能不能做这件事"。
func viewer(c *gin.Context) domain.Viewer {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil {
		return domain.Anonymous()
	}
	return domain.Viewer{
		ID:   domain.UserID(claims.UserID),
		Role: domain.Role(claims.Role),
	}
}

// userID 从 claims 取当前登录用户 ID。
func userID(c *gin.Context) domain.UserID { return viewer(c).ID }

// fail 把错误交给统一响应层：apperr 里有文案，未知错误按原样输出。
func fail(c *gin.Context, err error) {
	res.FailWithError(err, c)
}

// Detail 返回当前登录用户的详细资料。
// @Summary 当前用户详情
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[app.DetailView]
// @Router /users/detail [get]
func (h *Handler) Detail(c *gin.Context) {
	view, err := h.svc.Detail(c.Request.Context(), viewer(c))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(view, c)
}
