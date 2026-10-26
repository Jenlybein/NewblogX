package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"

	analyticsch "myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/apperr"
	authhttp "myblogx/internal/auth/adapter/http"
	authmysql "myblogx/internal/auth/adapter/mysql"
	authredis "myblogx/internal/auth/adapter/redis"
	authsec "myblogx/internal/auth/adapter/security"
	authapp "myblogx/internal/auth/app"
	authdomain "myblogx/internal/auth/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	mediamysql "myblogx/internal/media/adapter/mysql"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/mail"
	"myblogx/internal/platform/oauth"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_email"
	"myblogx/internal/platform/redis/redis_user"
	socialdomain "myblogx/internal/social/domain"
	userhttp "myblogx/internal/user/adapter/http"
	usermysql "myblogx/internal/user/adapter/mysql"
	userredis "myblogx/internal/user/adapter/redis"
	usersec "myblogx/internal/user/adapter/security"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"
	"myblogx/utils/jwts"
	"myblogx/utils/requestmeta"

	"gorm.io/gorm"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
)

// WireUser 装配 user 与 auth 两个模块，并注入认证路由处理器。
//
// 这两个模块一起装配：auth 需要 user 提供的凭证/账号能力，user 的注册建档又
// 依赖 auth 的会话语义，拆成两次装配只会让桥接对象没地方放。
// authService 供其他模块校验访问令牌（搜索等公开读接口用）。
var authService *authapp.Service

func WireUser(infra *Infra) {
	if infra == nil {
		return
	}
	cfg := infra.Config
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)

	// ── user 模块 ──
	usersRepo := usermysql.New(infra.DB)
	settingsRepo := usermysql.NewSettings(infra.DB)
	statsRepo := usermysql.NewStats(infra.DB)

	// authSvc 在 userSvc 之后才能构造（它要 userSvc 的凭证能力），
	// 但 userSvc 的"管理员改角色/状态要吊销会话"又需要 authSvc。
	// 用闭包延迟绑定打破这个环。
	var authSvc *authapp.Service
	sessionRevoker := userapp.SessionRevokerFunc(func(ctx context.Context, userID ctype.ID) error {
		if authSvc == nil {
			return nil
		}
		return authSvc.RevokeAll(ctx, userID)
	})

	userSvc := userapp.New(
		usersRepo,
		settingsRepo,
		statsRepo,
		usersRepo, // 快照同步也在同一个仓储上
		&userEnricher{db: infra.DB},
		realClock{},
		usersec.Hasher{},
		infra.Logger,
		userapp.WithUsernameAllocator(userredis.NewUsernameAllocator(redisOptions)),
		userapp.WithIntegrations(userapp.Integrations{
			Relation:  &relationBridge{},
			HomeView:  &homeViewBridge{db: infra.DB, redis: redisOptions, logger: infra.Logger},
			Sessions:  sessionRevoker,
			LastLogin: &lastLoginBridge{opts: analyticsLogOptions(infra)},
			Tags:      &tagBridge{db: infra.DB},
		}),
	)

	// ── auth 模块 ──
	sessionRepo := authmysql.NewSessionRepo(infra.DB)
	loginConf := func() siteconf.Login { return infra.RuntimeSite.GetRuntimeLogin() }

	authSvc = authapp.New(
		sessionRepo,
		&jwtIssuer{cfg: cfg.Jwt},
		authsec.NewSecrets(),
		authsec.NewHasher(),
		authredis.NewBlacklist(infra.Redis),
		authredis.NewLoginGuard(redisOptions, loginConf),
		&credentialBridge{svc: userSvc},
		realClock{},
		infra.Logger,
		authapp.Options{
			AccessTTL:  time.Duration(cfg.Jwt.Expire) * time.Hour,
			RefreshTTL: time.Duration(cfg.Jwt.RefreshExpire) * time.Hour,
		},
		authapp.WithDirectory(&directoryBridge{svc: userSvc}),
		authapp.WithEmailCodes(&emailCodeBridge{
			users:        usersRepo,
			conf:         cfg.Email,
			redisOptions: redisOptions,
			loginConf:    loginConf,
			logger:       infra.Logger,
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
	authService = authSvc
	router.SetAuthHandler(h)
	router.SetUserHandler(userhttp.NewHandler(userSvc))
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
		UserID:       c.UserID,
		SessionID:    c.SessionID,
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
		UserID:       mc.UserID,
		SessionID:    mc.SessionID,
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
	u, err := b.svc.FindForAuth(ctx, ctype.NewID(userID))
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
	return b.svc.ChangePassword(ctx, ctype.NewID(userID), oldPassword, newPassword)
}

// BindEmail 实现 authapp.Directory。
func (b *directoryBridge) BindEmail(ctx context.Context, userID uint64, email string) error {
	return b.svc.BindEmail(ctx, ctype.NewID(userID), email)
}

// emailCodeBridge 实现 authapp.EmailCodes：频控 + 按场景决定是否真的发信。
//
// 「是否真的发」是按旧实现的刻意设计：注册时邮箱已存在就不发，但接口照样返回
// 成功和 id——否则任何人都能拿这个接口批量探测哪些邮箱已注册。
type emailCodeBridge struct {
	users        userdomain.Repository
	conf         conf.Email
	redisOptions redis_service.Options
	loginConf    func() siteconf.Login
	logger       *logrus.Logger
}

var _ authapp.EmailCodes = (*emailCodeBridge)(nil)

// Send 实现 authapp.EmailCodes。
func (b *emailCodeBridge) Send(ctx context.Context, kind int8, email, ip string) (string, error) {
	lc := b.loginConf()
	if !redis_user.AllowEmailSend(b.redisOptions, lc, email, ip, kind) {
		return "", apperr.New(apperr.CodeAuthTooFrequent)
	}

	exists, err := b.users.ExistsEmail(ctx, email, "")
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
			err = mail.SendRegisterCode(b.conf, email, code, timeout)
		}
	case 2: // 重置密码：邮箱必须存在
		shouldSend = exists
		if shouldSend {
			err = mail.SendResetPwdCode(b.conf, email, code, timeout)
		}
	case 3: // 绑定邮箱：邮箱必须未被占用
		shouldSend = !exists
		if shouldSend {
			err = mail.SendBindEmailCode(b.conf, email, code, timeout)
		}
	case 4: // 邮箱登录：邮箱必须存在
		shouldSend = exists
		if shouldSend {
			err = mail.SendLoginCode(b.conf, email, code, timeout)
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
		if err := redis_email.Store(b.redisOptions, id, email, code, timeout, 3); err != nil {
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
	info, err := oauth.GetQQUserInfo(b.conf, code)
	if err != nil {
		return authapp.OAuthUser{}, err
	}
	return authapp.OAuthUser{OpenID: info.OpenID, Nickname: info.NickName, Avatar: info.Avatar}, nil
}

// policyBridge 实现 authhttp.LoginPolicy。
type policyBridge struct {
	site *sitecfg.RuntimeConfigService
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
	middleware.EmitLoginEventFromGin(c, event, loginTypeOf(kind), success, username, ctype.NewID(userID), reason, nil)
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
		ID:           u.ID,
		Username:     u.Username,
		Role:         int8(u.Role),
		TokenVersion: u.TokenVersion,
	}
}

// userEnricher 实现 app.DetailEnricher：头像反查 + 标签标题。
//
// 放装配层：它要直接读图片表与标签表，而业务模块禁止 import models。
type userEnricher struct {
	db *gorm.DB
}

// AvatarIDByURL 由头像 URL 反查图片 ID。
func (e *userEnricher) AvatarIDByURL(_ context.Context, url string) (ctype.ID, bool, error) {
	id, ok, err := mediamysql.FindIDByURL(e.db, url)
	return ctype.NewID(id), ok, err
}

// AvatarURLByID 由图片 ID 取 URL。
func (e *userEnricher) AvatarURLByID(_ context.Context, id ctype.ID) (string, error) {
	return mediamysql.ResolveURLByID(e.db, id.Uint64())
}

// TagTitles 批量取标签标题。
func (e *userEnricher) TagTitles(_ context.Context, ids []ctype.ID) (map[ctype.ID]string, error) {
	out := make(map[ctype.ID]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	// 标签表的主键仍是整型，查询前转回数值；结果再按 ID 语义返回给用例层。
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.Uint64())
	}
	var rows []tagTitleRow
	if err := e.db.Select("id", "title").Find(&rows, "id IN ?", raw).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Title
	}
	return out, nil
}

// relationBridge 实现 app.RelationReader（数据来自 social 模块）。
type relationBridge struct{}

// Relation 读关注关系。
func (b *relationBridge) Relation(ctx context.Context, viewer, target userdomain.UserID) (int8, error) {
	if socialService == nil {
		return socialdomain.RelationStranger.Int8(), nil
	}
	rel, err := socialService.Relation(ctx, viewer.Uint64(), target.Uint64())
	if err != nil {
		return socialdomain.RelationStranger.Int8(), nil
	}
	return rel.Int8(), nil
}

// homeViewBridge 实现 app.HomeViewRecorder：主页浏览按天去重计数。
//
// 两段式：Redis 先做当天挡板（便宜、能扛住热点主页的重复访问），
// 数据库唯一索引兜底（Redis 挂了或被清空时仍然不会重复计数）。
// 事务失败要把 Redis 的标记回滚掉，否则用户当天第二次真正访问会被误判为"已看过"。
type homeViewBridge struct {
	db     *gorm.DB
	redis  redis_service.Options
	logger *logrus.Logger
}

// Record 记录一次浏览，返回本次是否真正计入。
func (b *homeViewBridge) Record(_ context.Context, target, viewer userdomain.UserID) (bool, error) {
	if b.db == nil {
		return false, apperr.New(apperr.CodeCommonDBRequired)
	}
	targetID, viewerID := target.Uint64(), viewer.Uint64()
	if targetID == 0 || viewerID == 0 || targetID == viewerID {
		return false, nil
	}

	now := time.Now()
	viewDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Redis 挡板：能挡住绝大多数重复访问。
	reservedByRedis := false
	marked, markErr := redis_user.TryMarkUserHomeViewed(b.redis, ctype.NewID(targetID), ctype.NewID(viewerID), now)
	if markErr != nil {
		// 降级到数据库兜底，不因为缓存不可用就丢掉计数。
		if b.logger != nil {
			b.logger.Warnf("主页浏览 Redis 判重失败，降级走数据库: target=%d viewer=%d err=%v", targetID, viewerID, markErr)
		}
	} else {
		if !marked {
			return false, nil
		}
		reservedByRedis = true
	}

	counted := false
	err := b.db.Transaction(func(tx *gorm.DB) error {
		var err error
		counted, err = usermysql.RecordViewDaily(tx, targetID, viewerID, viewDate)
		return err
	})
	if err != nil && reservedByRedis {
		if rollbackErr := redis_user.RollbackUserHomeViewed(b.redis, ctype.NewID(targetID), ctype.NewID(viewerID), now); rollbackErr != nil {
			if b.logger != nil {
				b.logger.Warnf("主页浏览 Redis 判重回滚失败: target=%d viewer=%d err=%v", targetID, viewerID, rollbackErr)
			}
		}
	}
	return counted, err
}

// lastLoginBridge 实现 app.LastLoginReader。
type lastLoginBridge struct {
	opts analyticsch.Options
}

// Latest 批量读最后登录信息。
func (b *lastLoginBridge) Latest(_ context.Context, ids []uint64) (map[uint64]userapp.LastLogin, error) {
	out := make(map[uint64]userapp.LastLogin, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, ctype.NewID(id))
	}
	got, err := analyticsch.LoadLatestLoginMap(b.opts, raw)
	if err != nil {
		return nil, err
	}
	for id, rec := range got {
		at, _ := time.ParseInLocation("2006-01-02 15:04:05.000", rec.TS, time.Local)
		out[id.Uint64()] = userapp.LastLogin{IP: rec.IP, Addr: rec.Addr, At: at}
	}
	return out, nil
}

// tagBridge 实现 app.TagValidator。
type tagBridge struct {
	db *gorm.DB
}

// Validate 校验标签都存在且启用。
func (b *tagBridge) Validate(_ context.Context, ids []ctype.ID) error {
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.Uint64())
	}
	normalized := normalizeTagIDs(raw)
	if len(normalized) == 0 {
		return nil
	}
	var count int64
	if err := b.db.Table(tagTitleRow{}.TableName()).
		Where("id IN ? AND is_enabled = ?", normalized, true).
		Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(normalized)) {
		return apperr.New(apperr.CodeTagDisabled)
	}
	return nil
}

// normalizeTagIDs 去零去重。
func normalizeTagIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
