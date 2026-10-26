// Package http 是认证模块的 HTTP 适配器。
//
// 只做三件事：绑参数、调用用例、翻译响应。这里不出现 SQL，也不碰模型包
// （archcheck 的 mod-no-models 会拦），需要的枚举/角色都以原始类型传入。
package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/auth/app"
	"myblogx/internal/auth/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// LoginPolicy 是站点登录方式开关（运行时配置）。
type LoginPolicy interface {
	PasswordEnabled() bool
	EmailEnabled() bool
	QQEnabled() bool
}

// LoginEmitter 记录登录/登出事件。
//
// 用字符串 kind 而不是 models/enum 的数字枚举：认证适配器不能 import models
// （mod-no-models），映射到枚举的活儿放在装配层。
type LoginEmitter interface {
	Login(c *gin.Context, event, kind string, success bool, username string, userID uint64, reason string)
}

// MetaFunc 从请求提取 IP/地域/UA。
type MetaFunc func(c *gin.Context) domain.Meta

// Handler 是认证 HTTP 处理器。
type Handler struct {
	svc          *app.Service
	policy       LoginPolicy
	meta         MetaFunc
	emitter      LoginEmitter
	secureCookie bool
	refreshTTL   int64 // 小时
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service, policy LoginPolicy, meta MetaFunc, emitter LoginEmitter, secureCookie bool, refreshTTLHours int64) *Handler {
	if svc == nil {
		panic("auth/http: 用例服务未注入")
	}
	return &Handler{
		svc:          svc,
		policy:       policy,
		meta:         meta,
		emitter:      emitter,
		secureCookie: secureCookie,
		refreshTTL:   refreshTTLHours,
	}
}

func (h *Handler) passwordEnabled() bool { return h.policy == nil || h.policy.PasswordEnabled() }
func (h *Handler) emailEnabled() bool    { return h.policy == nil || h.policy.EmailEnabled() }
func (h *Handler) qqEnabled() bool       { return h.policy == nil || h.policy.QQEnabled() }

func (h *Handler) sessionMeta(c *gin.Context) domain.Meta {
	if h.meta == nil {
		return domain.Meta{}
	}
	return h.meta(c)
}

func (h *Handler) emit(c *gin.Context, event, kind string, success bool, username string, userID uint64, reason string) {
	if h.emitter != nil {
		h.emitter.Login(c, event, kind, success, username, userID, reason)
	}
}

func (h *Handler) cookieTTL() int64 {
	if h.refreshTTL <= 0 {
		return 30 * 24
	}
	return h.refreshTTL
}

func fail(c *gin.Context, err error) { res.FailWithError(err, c) }

// failBind 统一参数错误响应。
func failBind(c *gin.Context) { res.FailWithMsg("参数错误", c) }

// claims 取当前登录态；没登录返回 nil。
func claims(c *gin.Context) *jwts.MyClaims { return jwts.GetClaimsByGin(c) }

// PwdLogin 用户名/邮箱 + 密码登录。
// @Summary 密码登录
// @Tags user
// @Accept json
// @Produce json
// @Param body body PwdLoginRequest true "请求体"
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/login [post]
func (h *Handler) PwdLogin(c *gin.Context) {
	if !h.passwordEnabled() {
		h.emit(c, "login_fail", "password", false, "", 0, "站点未启用密码登录")
		res.FailWithMsg("站点未启用密码登录功能", c)
		return
	}
	var req PwdLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	result, err := h.svc.Login(c.Request.Context(), app.LoginInput{
		Account:  req.Username,
		Password: req.Password,
		Meta:     h.sessionMeta(c),
	})
	if err != nil {
		h.emit(c, "login_fail", "password", false, req.Username, 0, err.Error())
		fail(c, err)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "login_success", "password", true, result.Account.Username, result.Account.ID.Uint64(), "")
	res.OkWithData(result.Tokens.Access, c)
}

// Refresh 用刷新令牌换新的双令牌。
// @Summary 刷新访问令牌
// @Tags user
// @Produce json
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	token := refreshFromRequest(c.Request)
	result, err := h.svc.Refresh(c.Request.Context(), token, h.sessionMeta(c))
	if err != nil {
		clearRefreshCookie(c.Writer, h.secureCookie)
		h.emit(c, "token_refresh", "token", false, "", 0, err.Error())
		fail(c, err)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "token_refresh", "token", true, "", 0, "")
	res.OkWithData(result.Tokens.Access, c)
}

// Logout 退出当前设备。
// @Summary 退出登录
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	principal := &domain.Principal{
		UserID:    cl.UserID,
		SessionID: cl.SessionID,
		Username:  cl.Username,
		Role:      int8(cl.Role),
	}
	if err := h.svc.Logout(c.Request.Context(), principal, jwts.GetTokenByGin(c)); err != nil {
		h.emit(c, "logout", "logout", false, cl.Username, cl.UserID.Uint64(), err.Error())
		fail(c, err)
		return
	}
	clearRefreshCookie(c.Writer, h.secureCookie)
	h.emit(c, "logout", "logout", true, cl.Username, cl.UserID.Uint64(), "")
	res.OkWithMsg("退出登录成功", c)
}

// LogoutAll 退出全部设备。
// @Summary 退出全部设备
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/logout/all [post]
func (h *Handler) LogoutAll(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	if err := h.svc.RevokeAll(c.Request.Context(), cl.UserID); err != nil {
		h.emit(c, "logout_all", "logout", false, cl.Username, cl.UserID.Uint64(), err.Error())
		fail(c, err)
		return
	}
	clearRefreshCookie(c.Writer, h.secureCookie)
	h.emit(c, "logout_all", "logout", true, cl.Username, cl.UserID.Uint64(), "")
	res.OkWithMsg("已退出全部设备", c)
}

// Sessions 登录设备列表。
// @Summary 登录会话列表
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[SessionItem]]
// @Router /users/sessions [get]
func (h *Handler) Sessions(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	list, err := h.svc.ListSessions(c.Request.Context(), cl.UserID)
	if err != nil {
		fail(c, err)
		return
	}
	current := cl.SessionID.Uint64()
	items := make([]SessionItem, 0, len(list))
	for _, s := range list {
		items = append(items, SessionItem{
			ID:         s.ID,
			IP:         s.IP,
			Addr:       s.Addr,
			UA:         s.UA,
			CreatedAt:  s.CreatedAt,
			LastSeenAt: s.LastSeenAt,
			ExpiresAt:  s.ExpiresAt,
			IsCurrent:  s.ID.Uint64() == current,
		})
	}
	res.OkWithList(items, len(items), c)
}

// DeleteSession 下线指定设备。
// @Summary 下线指定会话
// @Tags user
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/sessions/{id} [delete]
func (h *Handler) DeleteSession(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		failBind(c)
		return
	}
	if err := h.svc.RevokeSession(c.Request.Context(), cl.UserID.Uint64(), id); err != nil {
		fail(c, err)
		return
	}
	if id == cl.SessionID.Uint64() {
		if token := jwts.GetTokenByGin(c); token != "" {
			_ = h.svc.Logout(c.Request.Context(), &domain.Principal{
				UserID: cl.UserID, SessionID: ctype.NewID(id), Username: cl.Username, Role: int8(cl.Role),
			}, token)
		}
		clearRefreshCookie(c.Writer, h.secureCookie)
		res.OkWithMsg("当前设备已下线", c)
		return
	}
	res.OkWithMsg("设备已下线", c)
}

// SendEmail 发送邮箱验证码。
// @Summary 发送邮箱验证码
// @Tags user
// @Accept json
// @Produce json
// @Param body body SendEmailRequest true "请求体"
// @Success 200 {object} kernel.Envelope[SendEmailResponse]
// @Router /users/email/verify [post]
func (h *Handler) SendEmail(c *gin.Context) {
	if !h.emailEnabled() {
		res.FailWithMsg("站点未启用邮箱功能", c)
		return
	}
	var req SendEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	id, err := h.svc.SendEmailCode(c.Request.Context(), req.Type, req.Email, c.ClientIP())
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithData(SendEmailResponse{ID: id}, c)
}

// EmailLogin 邮箱验证码登录（验证码已由中间件校验）。
// @Summary 邮箱验证码登录
// @Tags user
// @Produce json
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/email/login [post]
func (h *Handler) EmailLogin(c *gin.Context) {
	if !h.emailEnabled() {
		h.emit(c, "login_fail", "email", false, "", 0, "站点未启用邮箱登录")
		res.FailWithMsg("站点未启用邮箱登录功能", c)
		return
	}
	email := c.GetString("email")
	result, err := h.svc.LoginWithEmail(c.Request.Context(), email, h.sessionMeta(c))
	if err != nil {
		h.emit(c, "login_fail", "email", false, email, 0, err.Error())
		res.FailWithMsg("邮箱登录失败", c)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "login_success", "email", true, result.Account.Username, result.Account.ID.Uint64(), "")
	res.OkWithData(result.Tokens.Access, c)
}

// RegisterEmail 邮箱注册并登录。
// @Summary 邮箱注册
// @Tags user
// @Accept json
// @Produce json
// @Param body body RegisterEmailRequest true "请求体"
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/email/register [post]
func (h *Handler) RegisterEmail(c *gin.Context) {
	if !h.emailEnabled() {
		res.FailWithMsg("站点未启用邮箱注册功能", c)
		return
	}
	var req RegisterEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	email := c.GetString("email")
	result, err := h.svc.RegisterWithEmail(c.Request.Context(), email, req.Pwd, h.sessionMeta(c))
	if err != nil {
		h.emit(c, "register_fail", "email", false, email, 0, err.Error())
		fail(c, err)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "register_success", "email", true, result.Account.Username, result.Account.ID.Uint64(), "")
	h.emit(c, "login_success", "email", true, result.Account.Username, result.Account.ID.Uint64(), "")
	res.OkWithData(result.Tokens.Access, c)
}

// QQLogin QQ 授权码登录。
// @Summary QQ 登录
// @Tags user
// @Accept json
// @Produce json
// @Param body body QQLoginRequest true "请求体"
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/qq [post]
func (h *Handler) QQLogin(c *gin.Context) {
	if !h.qqEnabled() {
		h.emit(c, "login_fail", "qq", false, "", 0, "站点未启用QQ登录")
		res.FailWithMsg("站点未启用qq登录功能", c)
		return
	}
	var req QQLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	result, err := h.svc.LoginWithQQ(c.Request.Context(), req.Code, h.sessionMeta(c))
	if err != nil {
		h.emit(c, "login_fail", "qq", false, "", 0, err.Error())
		fail(c, err)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "login_success", "qq", true, result.Account.Username, result.Account.ID.Uint64(), "")
	res.OkWithData(result.Tokens.Access, c)
}

// ResetPassword 邮箱验证码重置密码。
// @Summary 邮箱重置密码
// @Tags user
// @Accept json
// @Produce json
// @Param body body ResetPasswordRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/password/recovery/email [put]
func (h *Handler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := h.svc.ResetPasswordByEmail(c.Request.Context(), c.GetString("email"), req.NewPassword); err != nil {
		fail(c, err)
		return
	}
	clearRefreshCookie(c.Writer, h.secureCookie)
	res.OkWithMsg("密码重置成功", c)
}

// UpdatePassword 已登录用户改密码。
// @Summary 修改密码
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body UpdatePasswordRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/password/renewal/email [put]
func (h *Handler) UpdatePassword(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), cl.UserID.Uint64(), req.OldPassword, req.NewPassword); err != nil {
		fail(c, err)
		return
	}
	clearRefreshCookie(c.Writer, h.secureCookie)
	res.OkWithMsg("密码更新成功", c)
}

// BindEmail 绑定邮箱（验证码已由中间件校验）。
// @Summary 绑定邮箱
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/email/bind [put]
func (h *Handler) BindEmail(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	if err := h.svc.BindEmail(c.Request.Context(), cl.UserID.Uint64(), c.GetString("email")); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("邮箱绑定成功", c)
}

// cookieDuration 把小时换成时长。
func (h *Handler) cookieDuration() time.Duration {
	return time.Duration(h.cookieTTL()) * time.Hour
}
