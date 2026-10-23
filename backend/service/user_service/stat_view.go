package user_service

import (
	"fmt"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_user"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StatRecordUserHomeView 记录一次“登录用户访问他人主页”的去重浏览。
// 返回 counted=true 表示本次访问真正让 ViewCount +1。
func StatRecordUserHomeView(deps Deps, userID, viewerUserID ctype.ID) (counted bool, err error) {
	if deps.DB == nil {
		return false, ErrAuthInvalid
	}

	if userID.IsZero() || viewerUserID.IsZero() || userID == viewerUserID {
		return false, nil
	}

	now := time.Now()
	viewDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Redis 先做当天去重挡板；Redis 不可用时降级到数据库唯一索引兜底。
	reservedByRedis := false
	marked, markErr := redis_user.TryMarkUserHomeViewed(deps.Redis, userID, viewerUserID, now)
	if markErr != nil {
		if deps.Logger != nil {
			deps.Logger.Warnf("记录用户主页访问时 Redis 判重失败，降级走数据库兜底: user_id=%d viewer_user_id=%d err=%v", userID, viewerUserID, markErr)
		}
	} else {
		if !marked {
			return false, nil
		}
		reservedByRedis = true
	}

	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		row := models.UserViewDailyModel{
			UserID:       userID,
			ViewerUserID: viewerUserID,
			ViewDate:     viewDate,
		}
		createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if createResult.Error != nil {
			return createResult.Error
		}
		if createResult.RowsAffected == 0 {
			return nil
		}

		if err := StatEnsureRows(tx, userID); err != nil {
			return err
		}
		updateResult := tx.Model(&models.UserStatModel{}).
			Where("user_id = ?", userID).
			Update("view_count", gorm.Expr("view_count + 1"))
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return fmt.Errorf("用户浏览统计更新失败: user_id=%d", userID)
		}

		counted = true
		return nil
	})
	if err != nil && reservedByRedis {
		if rollbackErr := redis_user.RollbackUserHomeViewed(deps.Redis, userID, viewerUserID, now); rollbackErr != nil {
			if deps.Logger != nil {
				deps.Logger.Warnf("回滚用户主页访问 Redis 判重失败: user_id=%d viewer_user_id=%d err=%v", userID, viewerUserID, rollbackErr)
			}
		}
	}
	return counted, err
}
