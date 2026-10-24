package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"

	"myblogx/internal/apperr"
	authhttp "myblogx/internal/auth/adapter/http"
	authmysql "myblogx/internal/auth/adapter/mysql"
	authredis "myblogx/internal/auth/adapter/redis"
	authsec "myblogx/internal/auth/adapter/security"
	authapp "myblogx/internal/auth/app"
	authdomain "myblogx/internal/auth/domain"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_email"
	"myblogx/internal/platform/redis/redis_user"
	usermysql "myblogx/internal/user/adapter/mysql"
	userredis "myblogx/internal/user/adapter/redis"
	usersec "myblogx/internal/user/adapter/security"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/router"
	"myblogx/service/email_service"
	"myblogx/service/qq_service"
	"myblogx/service/site_service"
	"myblogx/utils/jwts"
	"myblogx/utils/requestmeta"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
)

// WireUser 装配 user 与 auth 两个模块，并注入认证路由处理器。
//
// 这两个模块一起装配：auth 需要 user 提供的凭证/账号能力，user 的注册建档又
// 依赖 auth 的会话语义，拆成两次装配只会让桥接对象没地方放。
func WireUser(infra *Infra) {
	if infra == nil {
		return
	}
	cfg := infra.Config
	redisDeps := redis_service.NewDeps(infra.Redis, infra.Logger)

	// ── user 模块 ──
	usersRepo := usermysql.New(infra.DB)
	settingsRepo := usermysql.NewSettings(infra.DB)
	statsRepo := usermysql.NewStats(infra.DB)
	userSvc := userapp.New(
		usersRepo,
		settingsRepo,
		statsRepo,
		usersRepo, // 快照同步也在同一个仓储上
		nil,       // 详情增强读（头像图片 ID / 标签标题）不影响认证，暂不注入
		realClock{},
		usersec.Hasher{},
		infra.Logger,
		userapp.WithUsernameAllocator(userredis.NewUsernameAllocator(redisDeps)),
	)

	// ── auth 模块 ──
	sessionRepo := authmysql.NewSessionRepo(infra.DB)
	loginConf := func() siteconf.Login { return infra.RuntimeSite.GetRuntimeLogin() }

	authSvc := authapp.New(
		sessionRepo,
		&jwtIssuer{cfg: cfg.Jwt},
		authsec.NewSecrets(),
		authsec.NewHasher(),
		authredis.NewBlacklist(infra.Redis),
		authredis.NewLoginGuard(redisDeps, loginConf),
		&credentialBridge{svc: userSvc},
		realClock{},
		infra.Logger,
		authapp.Options{
			AccessTTL:  time.Duration(cfg.Jwt.Expire) * time.Hour,
			RefreshTTL: time.Duration(cfg.Jwt.RefreshExpire) * time.Hour,
		},
		authapp.WithDirectory(&directoryBridge{svc: userSvc}),
		authapp.WithEmailCodes(&emailCodeBridge{
			users:     usersRepo,
			conf:      cfg.Email,
			redisDeps: redisDeps,
			loginConf: loginConf,
			logger:    infra.Logger,
		}),
		authapp.WithOAuth(qqBridge{conf: cfg.QQ}),
	)

	h := authhttp.NewHandler(
		authSvc,
		&policyBridge{site: infra.RuntimeSite},
		func(c *gin.Context) authdomain.Meta {
			m := requestmeta.BuildSessionMeta(c)
			return authdomain.Meta{IP: m.IP, Addr: m.Addr, UA: m.UA}
		},
		loginEmitter{},
		cfg.System.Env == "prod",
		cfg.Jwt.RefreshExpire,
	)
	router.SetAuthHandler(h)
}

// jwtIssuer 用既有 JWT 工具签发认证令牌，实现 authdomain.TokenIssuer。
//
// 放在装配层而不是 internal/auth：载荷里的角色枚举来自 models/enum，
// 业务模块禁止 import models（R10）。
type jwtIssuer struct {
	cfg conf.Jwt
}

var _ authdomain.TokenIssuer = (*jwtIssuer)(nil)

// Issue 签发访问令牌。
func (i *jwtIssuer) Issue(c authdomain.Claims) (string, error) {
	return jwts.GetToken(i.cfg, jwts.Claims{
		UserID:       ctype.ID(c.UserID),
		SessionID:    ctype.ID(c.SessionID),
		TokenVersion: c.TokenVersion,
		Username:     c.Username,
		Role:         enum.RoleType(c.Role),
	})
}

// Parse 解析并校验访问令牌。
func (i *jwtIssuer) Parse(token string) (authdomain.Claims, error) {
	mc, err := jwts.ParseToken(i.cfg, token)
	if err != nil {
		return authdomain.Claims{}, err
	}
	return authdomain.Claims{
		UserID:       uint64(mc.UserID),
		SessionID:    uint64(mc.SessionID),
		TokenVersion: mc.TokenVersion,
		Username:     mc.Username,
		Role:         int8(mc.Role),
	}, nil
}

// credentialBridge 把 user 的凭证能力适配成 auth 需要的 Account。
type credentialBridge struct {
	svc *userapp.Service
}

var _ authapp.Credentials = (*credentialBridge)(nil)

// VerifyPassword 实现 authapp.Credentials。
func (b *credentialBridge) VerifyPassword(ctx context.Context, identifier, plain string) (authapp.Account, error) {
	u, err := b.svc.VerifyPassword(ctx, identifier, plain)
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// FindAccount 实现 authapp.Credentials。
func (b *credentialBridge) FindAccount(ctx context.Context, userID uint64) (authapp.Account, error) {
	u, err := b.svc.FindForAuth(ctx, userdomain.UserID(userID))
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// directoryBridge 把 user 的账号生命周期能力适配成 auth 需要的 Directory。
type directoryBridge struct {
	svc *userapp.Service
}

var _ authapp.Directory = (*directoryBridge)(nil)

// FindByEmail 实现 authapp.Directory。
func (b *directoryBridge) FindByEmail(ctx context.Context, email string) (authapp.Account, error) {
	u, err := b.svc.FindByEmailForAuth(ctx, email)
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// RegisterByEmail 实现 authapp.Directory。
func (b *directoryBridge) RegisterByEmail(ctx context.Context, email, plainPassword string) (authapp.Account, error) {
	u, err := b.svc.RegisterByEmail(ctx, email, plainPassword)
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// FindOrCreateByOpenID 实现 authapp.Directory。
func (b *directoryBridge) FindOrCreateByOpenID(ctx context.Context, openID, nickname, avatar string) (authapp.Account, error) {
	u, err := b.svc.FindOrCreateByOpenID(ctx, openID, nickname, avatar)
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// ResetPasswordByEmail 实现 authapp.Directory。
func (b *directoryBridge) ResetPasswordByEmail(ctx context.Context, email, newPassword string) (authapp.Account, error) {
	u, err := b.svc.ResetPasswordByEmail(ctx, email, newPassword)
	if err != nil {
		return authapp.Account{}, err
	}
	return toAuthAccount(u), nil
}

// ChangePassword 实现 authapp.Directory。
func (b *directoryBridge) ChangePassword(ctx context.Context, userID uint64, oldPassword, newPassword string) error {
	return b.svc.ChangePassword(ctx, userdomain.UserID(userID), oldPassword, newPassword)
}

// BindEmail 实现 authapp.Directory。
func (b *directoryBridge) BindEmail(ctx context.Context, userID uint64, email string) error {
	return b.svc.BindEmail(ctx, userdomain.UserID(userID), email)
}

// emailCodeBridge 实现 authapp.EmailCodes：频控 + 按场景决定是否真的发信。
//
// 「是否真的发」是按旧实现的刻意设计：注册时邮箱已存在就不发，但接口照样返回
// 成功和 id——否则任何人都能拿这个接口批量探测哪些邮箱已注册。
type emailCodeBridge struct {
	users     userdomain.Repository
	conf      conf.Email
	redisDeps redis_service.Deps
	loginConf func() siteconf.Login
	logger    *logrus.Logger
}

var _ authapp.EmailCodes = (*emailCodeBridge)(nil)

// Send 实现 authapp.EmailCodes。
func (b *emailCodeBridge) Send(ctx context.Context, kind int8, email, ip string) (string, error) {
	lc := b.loginConf()
	if !redis_user.AllowEmailSend(b.redisDeps, lc, email, ip, kind) {
		return "", apperr.New(apperr.CodeAuthTooFrequent)
	}

	exists, err := b.users.ExistsEmail(ctx, email, 0)
	if err != nil {
		return "", err
	}

	timeout := lc.EmailCodeTimeout
	if timeout <= 0 {
		timeout = 5
	}
	code := base64Captcha.RandText(4, "0123456789")

	shouldSend := false
	switch kind {
	case 1: // 注册：邮箱必须不存在
		shouldSend = !exists
		if shouldSend {
			err = email_service.SendRegisterCode(b.conf, email, code, timeout)
		}
	case 2: // 重置密码：邮箱必须存在
		shouldSend = exists
		if shouldSend {
			err = email_service.SendResetPwdCode(b.conf, email, code, timeout)
		}
	case 3: // 绑定邮箱：邮箱必须未被占用
		shouldSend = !exists
		if shouldSend {
			err = email_service.SendBindEmailCode(b.conf, email, code, timeout)
		}
	case 4: // 邮箱登录：邮箱必须存在
		shouldSend = exists
		if shouldSend {
			err = email_service.SendLoginCode(b.conf, email, code, timeout)
		}
	default:
		return "", apperr.New(apperr.CodeUserMailSendFailed)
	}
	if err != nil {
		if b.logger != nil {
			b.logger.Errorf("邮件发送失败: to=%s err=%v", email, err)
		}
		return "", apperr.New(apperr.CodeUserMailSendFailed)
	}

	id := base64Captcha.RandomId()
	if shouldSend {
		if err := redis_email.Store(b.redisDeps, id, email, code, timeout, 3); err != nil {
			if b.logger != nil {
				b.logger.Errorf("邮件验证码存储失败: %v", err)
			}
			return "", apperr.New(apperr.CodeUserMailSendFailed)
		}
	}
	return id, nil
}

// qqBridge 实现 authapp.OAuth。
type qqBridge struct {
	conf conf.QQ
}

var _ authapp.OAuth = (*qqBridge)(nil)

// Exchange 用授权码换 QQ 用户信息。
func (b qqBridge) Exchange(_ context.Context, code string) (authapp.OAuthUser, error) {
	info, err := qq_service.GetUserInfo(b.conf, code)
	if err != nil {
		return authapp.OAuthUser{}, err
	}
	return authapp.OAuthUser{OpenID: info.OpenID, Nickname: info.NickName, Avatar: info.Avatar}, nil
}

// policyBridge 实现 authhttp.LoginPolicy。
type policyBridge struct {
	site *site_service.RuntimeConfigService
}

// PasswordEnabled 是否启用密码登录。
func (p *policyBridge) PasswordEnabled() bool {
	return p.site != nil && p.site.GetRuntimeLogin().UsernamePwdLogin
}

// EmailEnabled 是否启用邮箱登录。
func (p *policyBridge) EmailEnabled() bool {
	return p.site != nil && p.site.GetRuntimeLogin().EmailLogin
}

// QQEnabled 是否启用 QQ 登录。
func (p *policyBridge) QQEnabled() bool {
	return p.site != nil && p.site.GetRuntimeLogin().QQLogin
}

// loginEmitter 实现 authhttp.LoginEmitter。
type loginEmitter struct{}

// Login 记录登录事件。kind 到枚举的映射放在装配层，认证适配器不 import models。
func (loginEmitter) Login(c *gin.Context, event, kind string, success bool, username string, userID uint64, reason string) {
	middleware.EmitLoginEventFromGin(c, event, loginTypeOf(kind), success, username, ctype.ID(userID), reason, nil)
}

// loginTypeOf 把字符串类别映射成登录类型枚举。
func loginTypeOf(kind string) enum.LoginType {
	switch kind {
	case "password":
		return enum.PasswordLoginType
	case "email":
		return enum.EmailLoginType
	case "qq":
		return enum.QQLoginType
	default:
		return enum.LoginType(0)
	}
}

// toAuthAccount 把 user 实体裁剪成认证需要的五个字段。
func toAuthAccount(u userdomain.User) authapp.Account {
	return authapp.Account{
		ID:           u.ID.Uint64(),
		Username:     u.Username,
		Role:         int8(u.Role),
		TokenVersion: u.TokenVersion,
	}
}
