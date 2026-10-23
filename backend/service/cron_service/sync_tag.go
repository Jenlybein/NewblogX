package cron_service

import (
	"context"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_tag"

	"gorm.io/gorm"
)

const (
	tagSyncLockKey = "cron:sync_tag:lock"
	tagSyncLockTTL = 30 * time.Minute
)

func (s *CronService) SyncTag() {
	s.runLockedSyncTask("同步标签任务", tagSyncLockKey, tagSyncLockTTL, func(ctx context.Context) (int, error) {
		return s.syncHashCounterMetric(ctx, hashCounterSyncConfig{
			taskName:   "同步标签任务",
			metricName: "文章数",
			activeKey:  redis_tag.TagCacheArticleCount,
			syncKey:    "tag_article_count:syncing",
			idName:     "tag_id",
			applyDelta: s.applyTagArticleCountDeltaToDB,
		})
	})
}

func (s *CronService) applyTagArticleCountDeltaToDB(tagID ctype.ID, delta int) error {
	expr := "CASE WHEN article_count + ? < 0 THEN 0 ELSE article_count + ? END"
	db := s.db.Model(&models.TagModel{}).
		Where("id = ?", tagID).
		UpdateColumn("article_count", gorm.Expr(expr, delta, delta))

	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 && s.log != nil {
		s.log.Warnf("同步标签任务更新行不存在: 标签ID=%d 增量=%d", tagID, delta)
	}
	return nil
}
