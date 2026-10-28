// 首次初始化接口。
//
// 站点刚部署起来时库里一个管理员都没有，后台登录页进不去、也没人能创建
// 第一个管理员 —— 这是个先有鸡还是先有蛋的问题。这两个接口就是那道门：
//
//	GET  /api/setup/status  还需要初始化吗
//	POST /api/setup/admin   创建第一个管理员
//
// 闸门在用例层（app.SetupFirstAdmin 里的"库里有没有管理员"），不在这里：
// 接入层只负责解析与响应，判断放在能拦住所有入口的那一层。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/user/app"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// SetupStatusResponse 是初始化状态。
type SetupStatusResponse struct {
	// NeedsAdmin 为真表示库里还没有管理员，初始化入口开放。
	NeedsAdmin bool `json:"needs_admin"`
}

// SetupAdminRequest 是创建第一个管理员的请求体。
type SetupAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

// SetupAdminResponse 是创建结果。
//
// 只回 ID 与用户名：这个接口是公开的，回得越少越好。
// 尤其不能回角色以外的东西 —— 一旦未来有人在这里多带一个字段，
// 匿名请求就能读到库里的内容。
type SetupAdminResponse struct {
	ID       ctype.ID `json:"id"`
	Username string   `json:"username"`
}

// SetupStatus 返回站点是否还需要初始化。
// @Summary 查询首次初始化状态
// @Tags setup
// @Produce json
// @Success 200 {object} kernel.Envelope[SetupStatusResponse]
// @Router /setup/status [get]
func (h *Handler) SetupStatus(c *gin.Context) {
	status, err := h.svc.SetupStatus(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(SetupStatusResponse{NeedsAdmin: status.NeedsAdmin}, c)
}

// SetupAdmin 创建站点第一个管理员。
//
// 成功之后该接口永久关闭（用例层返回 setup.already_done），
// 所以即使地址泄漏也不会变成"任何人都能再建一个管理员"的后门。
// @Summary 创建第一个管理员（仅首次初始化可用）
// @Tags setup
// @Accept json
// @Produce json
// @Param body body SetupAdminRequest true "管理员账号"
// @Success 200 {object} kernel.Envelope[SetupAdminResponse]
// @Router /setup/admin [post]
func (h *Handler) SetupAdmin(c *gin.Context) {
	var req SetupAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithError(err, c)
		return
	}

	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}
	var email *string
	if req.Email != "" {
		value := req.Email
		email = &value
	}

	u, err := h.svc.SetupFirstAdmin(c.Request.Context(), app.CreateInput{
		Username: req.Username,
		Nickname: nickname,
		Password: req.Password,
		Email:    email,
		IP:       c.ClientIP(),
	})
	if err != nil {
		if apperr.IsCode(err, apperr.CodeSetupAlreadyDone) {
			res.FailWithMsg("站点已初始化完成，该入口已关闭", c)
			return
		}
		fail(c, err)
		return
	}

	res.OkWithData(SetupAdminResponse{ID: u.ID, Username: u.Username}, c)
	// 此刻还没有登录态，审计里的用户是空的 —— 但仍要留一条：
	// "站点是什么时候、从哪个 IP 完成初始化的"是事后必须能查到的事实。
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "setup_admin_create",
		TargetType: "user",
		TargetID:   u.ID.String(),
		Success:    true,
		Message:    "站点首次初始化，创建管理员: " + u.Username,
	})
}
