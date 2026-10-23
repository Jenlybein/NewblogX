package cron_service

import (
	"context"
	"fmt"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_comment"
	"time"

	"gorm.io/gorm"
)

const (
	commentDiggSyncLockKey = "cron:sync_comment_digg:lock"
	commentDiggSyncLockTTL = 30 * time.Minute
	commentDiggSyncingKey  = "comment_digg:syncing"
)

func (s *CronService) SyncCommentDigg() {
	s.runLockedSyncTask("同步评论点赞数任务", commentDiggSyncLockKey, commentDiggSyncLockTTL, func(ctx context.Context) (int, error) {
		return s.syncHashCounterMetric(ctx, hashCounterSyncConfig{
			taskName:   "同步评论点赞数任务",
			activeKey:  redis_comment.DiggCountCacheKey,
			syncKey:    commentDiggSyncingKey,
			idName:     "comment_id",
			applyDelta: s.applyCommentDiggDelta,
		})
	})
}

func (s *CronService) applyCommentDiggDelta(commentID ctype.ID, delta int) error {
	expr := fmt.Sprintf("CASE WHEN %s + ? < 0 THEN 0 ELSE %s + ? END", "digg_count", "digg_count")

	db := s.db.Model(&models.CommentModel{}).
		Where("id = ?", commentID).
		UpdateColumn("digg_count", gorm.Expr(expr, delta, delta))
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 && s.log != nil {
		s.log.Warnf("同步评论点赞数任务更新行不存在: 评论ID=%d 增量=%d", commentID, delta)
	}
	return nil
}
