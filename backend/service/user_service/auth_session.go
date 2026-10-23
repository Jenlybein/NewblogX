// Package user_service 用户认证与会话管理核心服务包
// 负责：登录令牌生成、访问令牌校验、刷新令牌、会话吊销、用户状态校验等核心鉴权逻辑
package user_service

import (
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_jwt"
	"myblogx/utils/jwts"
)

// refreshTokenCookieName 刷新令牌在Cookie中的键名
const refreshTokenCookieName = "refresh_token"

type Authenticator struct {
	DB     *gorm.DB
	Logger *logrus.Logger
	JWT    conf.Jwt
	Redis  redis_service.Deps
}

type SessionMeta struct {
	IP   string
	Addr string
	UA   string
}

type AuthResult struct {
	Token   string                   // 原始访问令牌
	Claims  *jwts.MyClaims           // 解析后的JWT自定义声明
	User    *models.UserModel        // 对应用户信息
	Session *models.UserSessionModel // 对应用户会话信息
}

func NewAuthenticator(db *gorm.DB, logger *logrus.Logger, jwt conf.Jwt, redisDeps redis_service.Deps) *Authenticator {
	return &Authenticator{
		DB:     db,
		Logger: logger,
		JWT:    jwt,
		Redis:  redisDeps,
	}
}

func (a *Authenticator) AuthenticateAccessToken(token string) (*AuthResult, error) {
	logger := a.Logger
	if token == "" {
		if logger != nil {
			logger.Warn("访问令牌鉴权失败: 访问令牌为空")
		}
		return nil, ErrAuthRequired
	}

	// 解析令牌
	claims, err := jwts.ParseToken(a.JWT, token)
	if err != nil {
		if logger != nil {
			logger.Warnf("访问令牌鉴权失败: 解析访问令牌失败，错误=%v", err)
		}
		return nil, ErrAuthInvalid
	}
	if claims.SessionID == 0 {
		if logger != nil {
			logger.Warnf("访问令牌鉴权失败: 会话ID为空，用户ID=%s", claims.UserID.String())
		}
		return nil, ErrAuthInvalid
	}

	// 校验令牌是否在Redis黑名单中
	if blackType, ok := redis_jwt.HasTokenBlack(a.Redis, token); !ok {
		if logger != nil {
			logger.Warnf(
				"访问令牌鉴权失败: 访问令牌命中黑名单或黑名单检查异常，用户ID=%s 会话ID=%s 原因=%s",
				claims.UserID.String(),
				claims.SessionID.String(),
				blackType.String(),
			)
		}
		return nil, ErrAuthInvalid
	}

	// 查询用户是否存在，并校验令牌版本
	user, err := a.loadAuthUser(claims.UserID)
	if err != nil {
		if logger != nil {
			logger.Warnf("访问令牌鉴权失败: 查询用户失败，用户ID=%s 错误=%v", claims.UserID.String(), err)
		}
		return nil, ErrAuthInvalid
	}
	if !user.CheckTokenVersion(claims.TokenVersion) {
		if logger != nil {
			logger.Warnf(
				"访问令牌鉴权失败: 令牌版本不匹配，用户ID=%s 令牌版本=%d 数据库版本=%d",
				claims.UserID.String(),
				claims.TokenVersion,
				user.TokenVersion,
			)
		}
		return nil, ErrAuthInvalid
	}

	// 校验用户状态
	if err = user.ValidateUserStatus(); err != nil {
		if logger != nil {
			logger.Warnf("访问令牌鉴权失败: 用户状态无效，用户ID=%s 状态=%d 错误=%v", user.ID.String(), user.Status, err)
		}
		return nil, err
	}

	// 校验会话是否有效（未吊销、未过期、归属正确）
	session, err := a.getSession(claims.SessionID, claims.UserID)
	if err != nil {
		if logger != nil {
			logger.Warnf(
				"访问令牌鉴权失败: 查询会话失败，用户ID=%s 会话ID=%s 错误=%v",
				claims.UserID.String(),
				claims.SessionID.String(),
				err,
			)
		}
		return nil, ErrAuthInvalid
	}

	// 补充声明信息
	claims.Role = user.Role
	claims.Username = user.Username
	claims.TokenVersion = user.TokenVersion

	// 认证成功，返回完整结果
	return &AuthResult{
		Token:   token,
		Claims:  claims,
		User:    user,
		Session: session,
	}, nil
}

func (a *Authenticator) AuthenticateSession(userID, sessionID ctype.ID) (*AuthResult, error) {
	logger := a.Logger
	if userID == 0 || sessionID == 0 {
		if logger != nil {
			logger.Warnf("会话鉴权失败: 用户ID或会话ID为空，用户ID=%s 会话ID=%s", userID.String(), sessionID.String())
		}
		return nil, ErrAuthInvalid
	}

	// 查询用户是否存在
	user, err := a.loadAuthUser(userID)
	if err != nil {
		if logger != nil {
			logger.Warnf("会话鉴权失败: 查询用户失败，用户ID=%s 错误=%v", userID.String(), err)
		}
		return nil, ErrAuthInvalid
	}
	// 校验用户状态（正常/禁用/封禁）
	if err := user.ValidateUserStatus(); err != nil {
		if logger != nil {
			logger.Warnf("会话鉴权失败: 用户状态无效，用户ID=%s 状态=%d 错误=%v", user.ID.String(), user.Status, err)
		}
		return nil, err
	}

	// 查询会话：必须有效、未吊销、未过期
	session, err := a.getSession(sessionID, userID)
	if err != nil {
		if logger != nil {
			logger.Warnf("会话鉴权失败: 查询会话失败，用户ID=%s 会话ID=%s 错误=%v", userID.String(), sessionID.String(), err)
		}
		return nil, ErrAuthInvalid
	}

	// 构造认证结果并返回
	return &AuthResult{
		Claims: &jwts.MyClaims{
			Claims: jwts.Claims{
				UserID:       user.ID,
				SessionID:    session.ID,
				TokenVersion: user.TokenVersion,
				Username:     user.Username,
				Role:         user.Role,
			},
		},
		User:    user,
		Session: session,
	}, nil
}

func (a *Authenticator) loadAuthUser(userID ctype.ID) (*models.UserModel, error) {
	var snapshot models.UserModel
	if err := a.DB.
		Select("id", "username", "role", "status", "token_version").
		Take(&snapshot, userID).Error; err != nil {
		return nil, err
	}

	return &snapshot, nil
}

func (a *Authenticator) getSession(sessionID, userID ctype.ID) (*models.UserSessionModel, error) {
	var session models.UserSessionModel
	if err := a.DB.
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, time.Now()).
		Take(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}
