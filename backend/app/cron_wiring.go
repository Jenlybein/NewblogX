package app

import (
	"context"
	"time"

	articlemysql "myblogx/internal/article/adapter/mysql"
	commentmysql "myblogx/internal/comment/adapter/mysql"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/redis/redis_comment"
	"myblogx/internal/platform/redis/redis_tag"
	"myblogx/internal/platform/rediscounter"
	"myblogx/internal/platform/scheduler"
)

// 计数刷盘的调度参数。
const (
	counterSyncInterval = 30 * time.Minute
	counterSyncLockTTL  = 30 * time.Minute

	// 每日日志清理的默认执行时间与保留天数（配置缺省时使用）。
	defaultLogCleanupAt    = "03:30:00"
	defaultLogRetentionDay = 7
)

// CounterMetric 描述"某个 Redis 计数 key 刷到某张表的某一列"。
//
// 这份映射是本文件的核心数据：key 名或列名写错，表现是"计数永远不涨/涨错列"，
// 而且不会报错。所以它被导出，由 cron_wiring_test 校验唯一性。
type CounterMetric struct {
	Task   string
	Metric string
	// ActiveKey 是业务写入的 Redis hash key。
	ActiveKey string
	// ArticleCol 是目标列（只有文章类指标用）。
	ArticleCol articlemysql.ArticleCounter
}

// SyncKey 是刷盘期间的临时 key。
//
// 从 ActiveKey 派生而不是手写：手写时四个指标复制粘贴很容易漏改一个，
// 结果两个指标共用同一个 sync key 互相覆盖 —— 而且不会报错，
// 只表现为"某个计数偶尔不涨"。派生之后这类错误不可能发生。
func (m CounterMetric) SyncKey() string { return m.ActiveKey + ":syncing" }

// ArticleCounterMetrics 是文章计数的四个指标。
//
// 四个指标共用一个任务与一把锁：写的都是同一张 article 表，
// 并发刷同一批文章没有收益，反而放大锁竞争。
var ArticleCounterMetrics = []CounterMetric{
	{Task: "同步文章计数", Metric: "收藏数", ActiveKey: string(redis_article.ArticleCacheFavorite), ArticleCol: articlemysql.ArticleFavorCount},
	{Task: "同步文章计数", Metric: "点赞数", ActiveKey: string(redis_article.ArticleCacheDigg), ArticleCol: articlemysql.ArticleDiggCount},
	{Task: "同步文章计数", Metric: "浏览数", ActiveKey: string(redis_article.ArticleCacheView), ArticleCol: articlemysql.ArticleViewCount},
	{Task: "同步文章计数", Metric: "评论数", ActiveKey: string(redis_article.ArticleCacheComment), ArticleCol: articlemysql.ArticleCommentCount},
}

// WireCron 注册并启动定时任务。
//
// 这里是唯一知道"哪个 Redis key 对应哪张表的哪一列"的地方：
//   - 刷盘机制在 platform/rediscounter（双缓冲 + 回补）
//   - 写入 SQL 在各自的 adapter（article / comment）
//   - 调度与互斥在 platform/scheduler
//
// 之前这些全在 service/cron_service 一个包里，既写 SQL 又管调度，
// 于是"改一个列名"和"调一次调度间隔"要在同一个文件里动。
func WireCron(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	sch, err := scheduler.New(scheduler.Options{
		Client: infra.Redis,
		Logger: infra.Logger,
	})
	if err != nil {
		return err
	}

	flusher := rediscounter.New(infra.Redis, infra.Logger)
	db := infra.DB

	if err := sch.Every(counterSyncInterval, scheduler.Job{
		Name:    "同步文章计数",
		LockKey: "cron:sync_article:lock",
		LockTTL: counterSyncLockTTL,
		Run: func(ctx context.Context) (int, error) {
			total := 0
			for _, m := range ArticleCounterMetrics {
				affected, err := flusher.Flush(ctx, rediscounter.Task{
					Name:      m.Task,
					Metric:    m.Metric,
					ActiveKey: m.ActiveKey,
					SyncKey:   m.SyncKey(),
					IDName:    "article_id",
					ApplyDelta: func(id uint64, delta int) error {
						hit, err := articlemysql.FlushArticleCount(db, m.ArticleCol, id, delta)
						if err != nil {
							return err
						}
						if !hit {
							// 文章已删除：丢弃这条增量，不回补。
							// 回补会让下一轮再撞同一行，永远刷不掉。
							infra.Logger.Warnf("同步文章计数: 文章不存在，丢弃增量 article_id=%d 列=%s 增量=%d", id, m.ArticleCol, delta)
						}
						return nil
					},
				})
				if err != nil {
					infra.Logger.Errorf("同步文章计数失败: 指标=%s 错误=%v", m.Metric, err)
					continue
				}
				total += affected
			}
			return total, nil
		},
	}); err != nil {
		return err
	}

	// ── 标签文章数 ──
	if err := sch.Every(counterSyncInterval, scheduler.Job{
		Name:    "同步标签文章数",
		LockKey: "cron:sync_tag:lock",
		LockTTL: counterSyncLockTTL,
		Run: func(ctx context.Context) (int, error) {
			return flusher.Flush(ctx, rediscounter.Task{
				Name:      "同步标签文章数",
				Metric:    "文章数",
				ActiveKey: redis_tag.TagCacheArticleCount,
				SyncKey:   redis_tag.TagCacheArticleCount + ":syncing",
				IDName:    "tag_id",
				ApplyDelta: func(id uint64, delta int) error {
					hit, err := articlemysql.FlushTagArticleCount(db, id, delta)
					if err != nil {
						return err
					}
					if !hit {
						infra.Logger.Warnf("同步标签文章数: 标签不存在，丢弃增量 tag_id=%d 增量=%d", id, delta)
					}
					return nil
				},
			})
		},
	}); err != nil {
		return err
	}

	// ── 评论的两个计数：回复数、点赞数 ──
	commentMetrics := []struct {
		name      string
		metric    string
		activeKey string
		syncKey   string
		lockKey   string
		col       commentmysql.CommentCounter
	}{
		{"同步评论回复数", "", redis_comment.ReplyCountCacheKey, "comment_reply:syncing", "cron:sync_comment_reply:lock", commentmysql.CommentReplyCount},
		{"同步评论点赞数", "", redis_comment.DiggCountCacheKey, "comment_digg:syncing", "cron:sync_comment_digg:lock", commentmysql.CommentDiggCount},
	}
	for _, m := range commentMetrics {
		m := m
		if err := sch.Every(counterSyncInterval, scheduler.Job{
			Name:    m.name,
			LockKey: m.lockKey,
			LockTTL: counterSyncLockTTL,
			Run: func(ctx context.Context) (int, error) {
				return flusher.Flush(ctx, rediscounter.Task{
					Name:      m.name,
					Metric:    m.metric,
					ActiveKey: m.activeKey,
					SyncKey:   m.syncKey,
					IDName:    "comment_id",
					ApplyDelta: func(id uint64, delta int) error {
						hit, err := commentmysql.FlushCommentCount(db, m.col, id, delta)
						if err != nil {
							return err
						}
						if !hit {
							infra.Logger.Warnf("%s: 评论不存在，丢弃增量 comment_id=%d 增量=%d", m.name, id, delta)
						}
						return nil
					},
				})
			},
		}); err != nil {
			return err
		}
	}

	// ── 每日日志清理 ──
	registerLogCleanup(infra, sch)

	sch.Start()
	return nil
}

// registerLogCleanup 按配置注册日志清理任务。
//
// 配置非法时不静默兜底成默认值就当成功：先记一条 Error，
// 让人知道"你配的那个时间没生效"，然后用默认时间跑。
func registerLogCleanup(infra *Infra, sch *scheduler.Scheduler) {
	cfg := infra.Config.Log.Cleanup
	if !cfg.Enabled {
		infra.Logger.Infof("日志清理任务未启用，跳过注册")
		return
	}

	retentionDays := cfg.RetentionDays
	if retentionDays < 1 {
		retentionDays = defaultLogRetentionDay
	}

	hour, minute, second, err := writer.ParseDailyAt(cfg.RunAt)
	if err != nil {
		infra.Logger.Errorf("日志清理时间配置非法，改用默认 %s: 错误=%v", defaultLogCleanupAt, err)
		hour, minute, second, _ = writer.ParseDailyAt(defaultLogCleanupAt)
	}

	if err := sch.Daily(hour, minute, second, scheduler.Job{
		Name: "日志清理",
		Run: func(ctx context.Context) (int, error) {
			return writer.CleanupExpiredLogs(infra.Config.Log.Dir, retentionDays, infra.Logger)
		},
	}); err != nil {
		infra.Logger.Errorf("注册日志清理任务失败: %v", err)
		return
	}
	infra.Logger.Infof("已注册日志清理任务: 保留天数=%d 每日执行时间=%02d:%02d:%02d",
		retentionDays, hour, minute, second)
}
