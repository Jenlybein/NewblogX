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
	commentReplySyncLockKey = "cron:sync_comment_reply:lock"
	commentReplySyncLockTTL = 30 * time.Minute
	commentReplySyncingKey  = "comment_reply:syncing"
)

func (s *CronService) SyncCommentReply() {
	s.runLockedSyncTask("同步评论回复数任务", commentReplySyncLockKey, commentReplySyncLockTTL, func(ctx context.Context) (int, error) {
		return s.syncHashCounterMetric(ctx, hashCounterSyncConfig{
			taskName:   "同步评论回复数任务",
			activeKey:  redis_comment.ReplyCountCacheKey,
			syncKey:    commentReplySyncingKey,
			idName:     "comment_id",
			applyDelta: s.applyCommentReplyDelta,
		})
	})
}

func (s *CronService) applyCommentReplyDelta(commentID ctype.ID, delta int) error {
	expr := fmt.Sprintf("CASE WHEN %s + ? < 0 THEN 0 ELSE %s + ? END", "reply_count", "reply_count")

	db := s.db.Model(&models.CommentModel{}).
		Where("id = ?", commentID).
		UpdateColumn("reply_count", gorm.Expr(expr, delta, delta))
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 && s.log != nil {
		s.log.Warnf("同步评论回复数任务更新行不存在: 评论ID=%d 增量=%d", commentID, delta)
	}
	return nil
}
