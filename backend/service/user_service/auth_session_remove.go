package user_service

import (
	"myblogx/models"
	"myblogx/models/ctype"
	"time"

	"gorm.io/gorm"
)

// RevokeSessionByID 吊销单个会话
// 用于：退出登录、指定设备下线
func RevokeSessionByID(deps Deps, userID, sessionID ctype.ID) error {
	if deps.DB == nil {
		return ErrAuthInvalid
	}
	now := time.Now()
	return deps.DB.
		Model(&models.UserSessionModel{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
		Updates(map[string]any{
			// 依赖 revoked_at 作为失效判定，避免把唯一索引列统一改成相同空值。
			"revoked_at": now,
		}).Error
}

// RevokeAllUserSessions 吊销用户所有会话
// 用于：用户修改密码、管理员强制踢人、全局注销
func RevokeAllUserSessions(deps Deps, userID ctype.ID) error {
	if deps.DB == nil {
		return ErrAuthInvalid
	}
	now := time.Now()
	return deps.DB.
		Model(&models.UserSessionModel{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{
			"revoked_at": now,
		}).Error
}

// UpdatePasswordAndRevokeSessions 修改密码并吊销所有会话
// 事务保证：密码更新 + 所有旧会话失效 原子性执行
func UpdatePasswordAndRevokeSessions(deps Deps, user *models.UserModel, hashedPassword string) error {
	if deps.DB == nil {
		return ErrAuthInvalid
	}
	now := time.Now()
	nextVersion := user.TokenVersion + 1 // 令牌版本+1，使所有旧AccessToken失效

	return deps.DB.Transaction(func(tx *gorm.DB) error {
		// 更新用户密码、令牌版本、密码修改时间
		if err := tx.Model(user).Updates(map[string]any{
			"password":                 hashedPassword,
			"token_version":            nextVersion,
			"last_password_changed_at": now,
		}).Error; err != nil {
			return err
		}

		// 吊销该用户所有会话
		if err := tx.Model(&models.UserSessionModel{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Updates(map[string]any{
				"revoked_at": now,
			}).Error; err != nil {
			return err
		}

		// 更新内存中的用户对象，保持一致
		user.TokenVersion = nextVersion
		user.Password = hashedPassword
		user.LastPasswordChangedAt = &now
		return nil
	})
}

// InvalidateUserAuthState 使用户所有认证状态失效
// 不修改密码，只吊销所有会话 + 升级令牌版本
// 用于：账号异常、风控、强制重新登录
func InvalidateUserAuthState(deps Deps, user *models.UserModel) error {
	if deps.DB == nil {
		return ErrAuthInvalid
	}
	now := time.Now()
	nextVersion := user.TokenVersion + 1

	return deps.DB.Transaction(func(tx *gorm.DB) error {
		// 升级令牌版本，使所有旧AccessToken失效
		if err := tx.Model(user).Updates(map[string]any{
			"token_version": nextVersion,
		}).Error; err != nil {
			return err
		}
		// 吊销所有会话
		if err := tx.Model(&models.UserSessionModel{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Updates(map[string]any{
				"revoked_at": now,
			}).Error; err != nil {
			return err
		}
		user.TokenVersion = nextVersion
		return nil
	})
}
