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
	"myblogx/internal/platform/mail"
	"myblogx/internal/platform/sms"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// LoginPolicy 是站点登录方式开关（运行时配置）。
type LoginPolicy interface {
	PasswordEnabled() bool
	EmailEnabled() bool
	// EmailRegisterEnabled 与 EmailEnabled 分开：注册与登录是两件事，
	// 而"绑定邮箱"要用发码接口。混成一个开关时，关掉邮箱注册会连带
	// 把给已注册用户发绑定验证码一起关掉。
	EmailRegisterEnabled() bool
	QQEnabled() bool
	// PhoneEnabled 是「手机号 + 短信验证码」登录（兼注册）开关。
	PhoneEnabled() bool
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
func (h *Handler) phoneEnabled() bool    { return h.policy == nil || h.policy.PhoneEnabled() }

func (h *Handler) emailRegisterEnabled() bool {
	return h.policy == nil || h.policy.EmailRegisterEnabled()
}

// emailPurposeEnabled 按验证码用途判定开关。
//
// 绑定邮箱**不依赖**邮箱登录/注册开关：它是"用手机号注册的人"之后补邮箱的
// 唯一入口，一旦跟着邮箱登录一起关掉，那些用户就再也绑不了邮箱。
func (h *Handler) emailPurposeEnabled(kind mail.Kind) bool {
	switch kind {
	case mail.KindRegister:
		return h.emailRegisterEnabled()
	case mail.KindResetPassword, mail.KindLogin:
		return h.emailEnabled()
	case mail.KindBindEmail:
		return true
	default:
		return false
	}
}

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
		// scope=admin：管理后台的登录入口，非管理员在服务层就被拒（不建会话、不发 cookie）。
		// 读者站不传 scope，行为与以前完全一致。
		RequireAdmin: req.Scope == "admin",
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
	list, err := h.svc.ListSessions(c.Request.Context(), cl.UserID, cl.SessionID)
	if err != nil {
		fail(c, err)
		return
	}
	current := cl.SessionID.Uint64()
	items := make([]SessionItem, 0, len(list))
	for _, d := range list {
		// 时间字段是指针，取副本地址避免每行共享同一个变量。
		lastSeen := d.LastSeenAt
		items = append(items, SessionItem{
			ID:             d.SessionID,
			Device:         d.Device.Label(),
			Browser:        d.Device.Browser,
			BrowserVersion: d.Device.BrowserVersion,
			OS:             d.Device.OS,
			OSVersion:      d.Device.OSVersion,
			DeviceType:     string(d.Device.Type),
			IP:             d.IP,
			Addr:           d.Addr,
			UA:             d.UA,
			CreatedAt:      d.FirstSeenAt,
			FirstSeenAt:    d.FirstSeenAt,
			LastSeenAt:     &lastSeen,
			ExpiresAt:      d.ExpiresAt,
			LoginCount:     d.LoginCount,
			// 当前设备标记靠会话 ID 比对：聚合行的 ID 就是代表会话。
			IsCurrent: d.SessionID.Uint64() == current,
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
	var req SendEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	// 按用途判定开关（不是先看一个总的 emailEnabled），理由见 emailPurposeEnabled。
	if !h.emailPurposeEnabled(mail.Kind(req.Type)) {
		res.FailWithMsg("站点未启用该邮箱功能", c)
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
	if !h.emailRegisterEnabled() {
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

// SendSms 发送手机验证码。
// @Summary 发送手机验证码
// @Tags user
// @Accept json
// @Produce json
// @Param body body SendSmsRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/phone/verify [post]
func (h *Handler) SendSms(c *gin.Context) {
	var req SendSmsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	// 只有"登录/注册"这条用途受 phone_login 开关管：它决定站点还收不收新的手机号用户。
	// 「设置首个密码」的复验是已登录用户证明自己是号码主人，站长不再收新用户
	// 不应该连带把老用户改密码也锁死（邮件那边的「绑定邮箱」同理不受开关管）。
	if sms.Kind(req.Type) == sms.KindAuth && !h.phoneEnabled() {
		res.FailWithMsg("站点未启用手机号登录", c)
		return
	}
	if err := h.svc.SendSmsCode(c.Request.Context(), req.Type, req.Phone, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("验证码已发送", c)
}

// PhoneLogin 手机号 + 短信验证码登录；首次登录自动注册。
//
// 核验放在 handler 里而不是中间件：短信验证码由阿里云生成并核验，
// 没有"一次性消费的本地记录"要在中间件里取，放中间件只是多一层间接。
// 只有核验通过的那个手机号才会进入用例层，请求体里的号码不单独信任。
// @Summary 手机号登录/注册
// @Tags user
// @Accept json
// @Produce json
// @Param body body PhoneLoginRequest true "请求体"
// @Success 200 {object} kernel.Envelope[string]
// @Router /users/phone [post]
func (h *Handler) PhoneLogin(c *gin.Context) {
	if !h.phoneEnabled() {
		h.emit(c, "login_fail", "phone", false, "", 0, "站点未启用手机号登录")
		res.FailWithMsg("站点未启用手机号登录功能", c)
		return
	}
	var req PhoneLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	ok, err := h.svc.CheckSmsCode(c.Request.Context(), req.Phone, req.Code)
	if err != nil {
		h.emit(c, "login_fail", "phone", false, "", 0, err.Error())
		fail(c, err)
		return
	}
	if !ok {
		h.emit(c, "login_fail", "phone", false, "", 0, "验证码错误")
		fail(c, apperr.New(apperr.CodeUserCaptchaInvalid))
		return
	}
	result, err := h.svc.LoginWithPhone(c.Request.Context(), req.Phone, h.sessionMeta(c))
	if err != nil {
		h.emit(c, "login_fail", "phone", false, "", 0, err.Error())
		fail(c, err)
		return
	}
	setRefreshCookie(c.Writer, result.Tokens.Refresh, h.cookieDuration(), h.secureCookie)
	h.emit(c, "login_success", "phone", true, result.Account.Username, result.Account.ID.Uint64(), "")
	res.OkWithData(result.Tokens.Access, c)
}

// SetPassword 给尚无密码的账号设置首个密码（需对绑定手机号再做一次短信复验）。
// @Summary 设置首个密码
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SetPasswordRequest true "请求体"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /users/password/set [post]
func (h *Handler) SetPassword(c *gin.Context) {
	cl := claims(c)
	if cl == nil {
		fail(c, apperr.New(apperr.CodeAuthRequired))
		return
	}
	var req SetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), cl.UserID.Uint64(), req.Phone, req.Code, req.NewPassword); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("密码设置成功", c)
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
