// Package http 是用户模块的 HTTP 适配器。
//
// 只做三件事：绑定参数、调用用例、把结果/错误翻译成响应。
// 这里不允许出现 SQL（archcheck R1 会拦）。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/user/app"
	"myblogx/internal/user/domain"
	"myblogx/utils/jwts"
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
// @Success 200 {object} object{code=int,data=app.DetailView,msg=string}
// @Router /users/detail [get]
func (h *Handler) Detail(c *gin.Context) {
	view, err := h.svc.Detail(c.Request.Context(), viewer(c))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(view, c)
}

// PublicProfile 返回指定用户的公开信息。
// @Summary 用户公开主页
// @Tags user
// @Produce json
// @Param id path string true "用户 ID"
// @Success 200 {object} object{code=int,data=app.PublicProfile,msg=string}
// @Router /users/{id} [get]
func (h *Handler) PublicProfile(c *gin.Context) {
	uri := mustURI(c)
	if uri == nil {
		return
	}
	profile, err := h.svc.PublicProfile(c.Request.Context(), uri.ID)
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(profile, c)
}

// Update 更新当前用户资料。
// @Summary 更新用户资料
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=app.UpdateResult,msg=string}
// @Router /users [put]
func (h *Handler) Update(c *gin.Context) {
	req, ok := bindJSON[UpdateRequest](c)
	if !ok {
		return
	}
	res2, err := h.svc.Update(c.Request.Context(), viewer(c), userID(c), req.toDomain())
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(res2, c)
}

// UpdateSettings 更新当前用户偏好设置。
// @Summary 更新用户偏好
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=string,msg=string}
// @Router /users/settings [put]
func (h *Handler) UpdateSettings(c *gin.Context) {
	req, ok := bindJSON[SettingsRequest](c)
	if !ok {
		return
	}
	if err := h.svc.UpdateSettings(c.Request.Context(), viewer(c), userID(c), req.toApp()); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("更新成功", c)
}

// List 分页查询用户（管理端）。
// @Summary 用户列表
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=object{list=[]domain.User,total=int64},msg=string}
// @Router /users [get]
func (h *Handler) List(c *gin.Context) {
	req, ok := bindQuery[ListRequest](c)
	if !ok {
		return
	}
	page, err := h.svc.List(c.Request.Context(), viewer(c), req.toDomain())
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithList(page.Items, int(page.Total), c)
}

// Create 创建用户（管理端）。
// @Summary 创建用户
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=domain.User,msg=string}
// @Router /users [post]
func (h *Handler) Create(c *gin.Context) {
	req, ok := bindJSON[CreateRequest](c)
	if !ok {
		return
	}
	u, err := h.svc.Create(c.Request.Context(), viewer(c), req.toApp(c.ClientIP()))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(u, c)
}

// ChangeStatus 修改账号状态（管理端）。
// @Summary 修改账号状态
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户 ID"
// @Success 200 {object} object{code=int,data=string,msg=string}
// @Router /users/{id}/status [put]
func (h *Handler) ChangeStatus(c *gin.Context) {
	uri := mustURI(c)
	if uri == nil {
		return
	}
	req, ok := bindJSON[StatusRequest](c)
	if !ok {
		return
	}
	if err := h.svc.ChangeStatus(c.Request.Context(), viewer(c), uri.ID, domain.Status(req.Status)); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("操作成功", c)
}

// failInvalid 用于绑定失败：参数错不查库，直接返回。
func failInvalid(c *gin.Context) {
	res.FailWithError(apperr.New(apperr.CodeUserUsernameInvalid), c)
}
