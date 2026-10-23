package cron_service

import (
	"context"
	"fmt"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_article"
	"time"

	"gorm.io/gorm"
)

const (
	articleSyncLockKey = "cron:sync_article:lock"
	articleSyncLockTTL = 30 * time.Minute
)

// articleSyncMetric 描述一类需要同步的计数指标。
type articleSyncMetric struct {
	name      string
	activeKey string
	syncKey   string
	column    string
}

// SyncArticle 定时任务入口：把 Redis 增量同步回数据库。
func (s *CronService) SyncArticle() {
	metrics := []articleSyncMetric{
		{name: "收藏数", activeKey: string(redis_article.ArticleCacheFavorite), syncKey: "article_favorite:syncing", column: "favor_count"},
		{name: "点赞数", activeKey: string(redis_article.ArticleCacheDigg), syncKey: "article_digg:syncing", column: "digg_count"},
		{name: "浏览数", activeKey: string(redis_article.ArticleCacheView), syncKey: "article_view:syncing", column: "view_count"},
		{name: "评论数", activeKey: string(redis_article.ArticleCacheComment), syncKey: "article_comment:syncing", column: "comment_count"},
	}

	s.runLockedSyncTask("同步文章任务", articleSyncLockKey, articleSyncLockTTL, func(ctx context.Context) (int, error) {
		totalAffected := 0
		for _, metric := range metrics {
			metric := metric
			affected, err := s.syncHashCounterMetric(ctx, hashCounterSyncConfig{
				taskName:   "同步文章任务",
				metricName: metric.name,
				activeKey:  metric.activeKey,
				syncKey:    metric.syncKey,
				idName:     "article_id",
				applyDelta: func(articleID ctype.ID, delta int) error {
					return s.applyArticleDelta(metric.column, articleID, delta)
				},
			})
			if err != nil {
				if s.log != nil {
					s.log.Errorf("同步文章任务同步%s失败: Redis键=%s 错误=%v", metric.name, metric.activeKey, err)
				}
				continue
			}
			if affected > 0 {
				if s.log != nil {
					s.log.Infof("同步文章任务同步%s成功: Redis键=%s 影响数量=%d", metric.name, metric.activeKey, affected)
				}
			}
			totalAffected += affected
		}
		return totalAffected, nil
	})
}

// applyArticleDelta 对单篇文章执行增量更新。
func (s *CronService) applyArticleDelta(column string, articleID ctype.ID, delta int) error {
	// 使用 CASE 防止减到负数（如点赞/收藏取消过多）。
	expr := fmt.Sprintf("CASE WHEN %s + ? < 0 THEN 0 ELSE %s + ? END", column, column)

	// UpdateColumn 使用数据库表达式原子更新，避免先读后写竞争。
	db := s.db.Model(&models.ArticleModel{}).
		Where("id = ?", articleID).
		UpdateColumn(column, gorm.Expr(expr, delta, delta))

	// SQL 执行失败直接返回。
	if db.Error != nil {
		return db.Error
	}

	// 如果文章不存在，记录告警但不中断整批任务。
	if db.RowsAffected == 0 && s.log != nil {
		s.log.Warnf("同步文章任务更新行不存在: 文章ID=%d 字段=%s 增量=%d", articleID, column, delta)
	}
	return nil
}
