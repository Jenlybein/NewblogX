package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"myblogx/conf/site"
	articlemysql "myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain/viewhistory"
	chatmysql "myblogx/internal/chat/adapter/mysql"
	commentmysql "myblogx/internal/comment/adapter/mysql"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/redis/redis_comment"
	"myblogx/internal/platform/redis/redis_tag"
	"myblogx/internal/platform/rediscounter"
	"myblogx/internal/platform/scheduler"
)

// 并账任务的调度参数。
//
// 周期不在这里：它是运行期配置 site.reconcile.interval_minutes（#279），
// 唯一来源是 site.Reconcile.Interval()。本文件只留锁的持有时间。
const (
	counterSyncLockTTL = 30 * time.Minute

	// 每日日志清理的默认执行时间与保留天数（配置缺省时使用）。
	defaultLogCleanupAt    = "03:30:00"
	defaultLogRetentionDay = 7
)

// reconcileInterval 返回当前生效的并账周期。
//
// 每次注册时现读运行期配置（库里的 site.reconcile 段），缺省 30 分钟。
// 想改周期不必改代码；但它是**调度注册参数**，与「日志清理每天几点」同一口径：
// 改完要重启才生效（换周期要重新注册任务），这一点写进 ADR-0025。
func reconcileInterval(infra *Infra) time.Duration {
	if infra.RuntimeSite == nil {
		return site.Reconcile{}.Interval()
	}
	return infra.RuntimeSite.GetRuntimeSite().Reconcile.Interval()
}

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

// FlushArticleCounters 执行一次「文章计数并账」：把四个指标在 Redis 里的增量
// 刷回 article_models，返回各指标处理的条目数之和。
//
// 单独导出是为了让用例能驱动**这条真实的任务体**（sqlite + miniredis 即可验证
// 「重算覆盖 / 增量清空 / 读路径守恒」），而不必起 MySQL 容器；容器用例只守
// MySQL 才有的语义（affected rows 口径、真实索引）。
//
// 写库方式由列本身决定（见 articlemysql.ReconcileArticleCount）：digg / favor
// 按关系行 COUNT 重算覆盖，view / comment 仍是列 + 增量。并账瞬间同一次事件可能
// 被「COUNT」与「增量」各算一次，但覆盖写不累积、下一轮自愈——推理写在
// ReconcileArticleCount 的注释里。
//
// 覆盖面：刷盘由"哪个 id 还有增量"驱动，所以重算只发生在**本轮有增量的文章**上。
// 一篇再没人互动的文章不会被重算——这是 ADR-0025「代价」里认下的范围，不是漏项：
// 漂移会在下一次有人点赞 / 收藏它时随并账一起被纠正。
func FlushArticleCounters(ctx context.Context, db *gorm.DB, flusher *rediscounter.Flusher, log rediscounter.Logger) (int, error) {
	if flusher == nil {
		return 0, nil
	}
	total := 0
	for _, m := range ArticleCounterMetrics {
		affected, err := flusher.Flush(ctx, rediscounter.Task{
			Name:      m.Task,
			Metric:    m.Metric,
			ActiveKey: m.ActiveKey,
			SyncKey:   m.SyncKey(),
			IDName:    "article_id",
			ApplyDelta: func(id uint64, delta int) error {
				hit, err := articlemysql.ReconcileArticleCount(db, m.ArticleCol, id, delta)
				if err != nil {
					return err
				}
				if !hit {
					// 文章已删除：丢弃这条增量，不回补。
					// 回补会让下一轮再撞同一行，永远刷不掉。
					if log != nil {
						log.Warnf("同步文章计数: 文章不存在，丢弃增量 article_id=%d 列=%s 增量=%d", id, m.ArticleCol, delta)
					}
				}
				return nil
			},
		})
		if err != nil {
			if log != nil {
				log.Errorf("同步文章计数失败: 指标=%s 错误=%v", m.Metric, err)
			}
			continue
		}
		total += affected
	}
	return total, nil
}

// ReconcileChatUnread 执行一次「私信未读重算」：把每条活跃会话的 unread_count
// 覆盖成按消息行算出的真值，返回被写回的条数。
//
// 单独导出是为了让用例能驱动这条真实任务体（sqlite 即可验证「重算覆盖 /
// 尊重清空水位 / 排除用户级删除」），不必起 MySQL 容器。
//
// 与文章计数并账**共用同一个 30 分钟 tick**（见 WireCron），不新增调度器：
// ADR-0025 认下的「不新增定时任务」在这里同样成立。私信没有 Redis 增量源，
// 所以覆盖面是全部活跃会话，而不是"本轮有增量的会话"。
func ReconcileChatUnread(ctx context.Context, db *gorm.DB) (int, error) {
	return chatmysql.ReconcileAllChatUnread(ctx, db, 200)
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
//
// lifecycle 用来登记停机钩子：调度器必须在 DB/Redis 关闭**之前**停下，
// 否则在途任务的 SQL 会打到已关闭的连接上（日志里出现一堆无意义的报错）。
// Lifecycle 的 stop hook 是后进先出，而 main 里先注册了基础设施清理，
// 所以这里注册的会先执行。
func WireCron(infra *Infra, lifecycle *Lifecycle) error {
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

	// 周期只有一个来源：运行期配置 site.reconcile.interval_minutes（#279）。
	interval := reconcileInterval(infra)

	if err := sch.Every(interval, scheduler.Job{
		Name:    "同步文章计数",
		LockKey: "cron:sync_article:lock",
		LockTTL: counterSyncLockTTL,
		Run: func(ctx context.Context) (int, error) {
			// 两个并账共用一个 tick：文章计数（增量驱动）与私信未读（全量重算）。
			// 失败彼此隔离 —— 一个失败不该挡住另一个自愈。
			articleTotal, articleErr := FlushArticleCounters(ctx, db, flusher, infra.Logger)
			if articleErr != nil {
				infra.Logger.Errorf("同步文章计数失败: %v", articleErr)
			}
			chatTotal, chatErr := ReconcileChatUnread(ctx, db)
			if chatErr != nil {
				infra.Logger.Errorf("重算私信未读失败: %v", chatErr)
			}
			if articleErr != nil {
				return articleTotal, articleErr
			}
			if chatErr != nil {
				return articleTotal, chatErr
			}
			return articleTotal + chatTotal, nil
		},
	}); err != nil {
		return err
	}

	// ── 标签文章数 ──
	if err := sch.Every(interval, scheduler.Job{
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
		if err := sch.Every(interval, scheduler.Job{
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
						hit, err := commentmysql.FlushCommentCount(db, m.col, ctype.NewID(id), delta)
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

	// ── 每日浏览历史清理（90 天）──
	registerViewHistoryCleanup(infra, sch)

	if lifecycle != nil {
		lifecycle.OnStop("scheduler", func(context.Context) error {
			return sch.Shutdown()
		})
	}

	sch.Start()
	return nil
}

// 浏览历史清理的调度参数。
//
// 每天一次、固定在凌晨低峰，并与日志清理（03:30）错开：两个任务都要扫表，
// 撞在一起没有收益。保留天数不是配置项 —— 90 天是数据策略（见
// viewhistory.Retention 与决策清单的「已知取舍」），要做成配置得先改发布说明。
const (
	viewHistoryCleanupHour    = 4
	viewHistoryCleanupMinute  = 10
	viewHistoryCleanupLockTTL = 10 * time.Minute
)

// PurgeViewHistory 执行一次浏览历史清理，返回软删的行数。
//
// now 由调用方传入（调度任务执行时取 time.Now()，用例传固定时刻）：
// 保留期与边界（"恰好 90 天前"留到下一轮）因此可以被钉死。
//
// 软删：与用户"移出历史"同一语义，误清可回滚。
func PurgeViewHistory(ctx context.Context, db *gorm.DB, now time.Time) (int, error) {
	if db == nil {
		return 0, nil
	}
	affected, err := articlemysql.PurgeViewHistoryBefore(ctx, db, viewhistory.PurgeCutoff(now))
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// registerViewHistoryCleanup 注册每日浏览历史清理任务。
//
// 与日志清理不同，这个任务的"开关/天数"没有运行时配置：它跟随 cron 体系
// 每天跑一次，参数全部来自常量与 domain 的保留期（viewhistory.Retention）。
func registerViewHistoryCleanup(infra *Infra, sch *scheduler.Scheduler) {
	if err := sch.Daily(viewHistoryCleanupHour, viewHistoryCleanupMinute, 0, scheduler.Job{
		Name:    "浏览历史清理",
		LockKey: "cron:purge_view_history:lock",
		LockTTL: viewHistoryCleanupLockTTL,
		Run: func(ctx context.Context) (int, error) {
			affected, err := PurgeViewHistory(ctx, infra.DB, time.Now())
			if err != nil {
				infra.Logger.Errorf("浏览历史清理失败: %v", err)
				return 0, err
			}
			return affected, nil
		},
	}); err != nil {
		infra.Logger.Errorf("注册浏览历史清理任务失败: %v", err)
		return
	}
	infra.Logger.Infof("已注册浏览历史清理任务: 每日 %02d:%02d，保留 %d 天（软删 updated_at < now-90d）",
		viewHistoryCleanupHour, viewHistoryCleanupMinute, int(viewhistory.Retention/(24*time.Hour)))
}

// registerLogCleanup 按配置注册日志清理任务。
//
// 配置非法时不静默兜底成默认值就当成功：先记一条 Error，
// 让人知道"你配的那个时间没生效"，然后用默认时间跑。
func registerLogCleanup(infra *Infra, sch *scheduler.Scheduler) {
	// 开关与保留天数都**每次现读**运行时配置：这样在后台打开/关闭清理、
	// 调整保留天数都不必重启。只有"每天几点跑"仍取启动时的基线
	// —— 那是调度注册参数，改它需要重新 CreateJob。
	cfg := infra.Config.Log.Cleanup

	hour, minute, second, err := writer.ParseDailyAt(cfg.RunAt)
	if err != nil {
		infra.Logger.Errorf("日志清理时间配置非法，改用默认 %s: 错误=%v", defaultLogCleanupAt, err)
		hour, minute, second, _ = writer.ParseDailyAt(defaultLogCleanupAt)
	}

	if err := sch.Daily(hour, minute, second, scheduler.Job{
		Name: "日志清理",
		Run: func(ctx context.Context) (int, error) {
			liveLog := infra.RuntimeSite.GetRuntimeLog()
			// 未启用时返回 0：调度器只在 affected>0 时打日志，所以不会刷屏。
			if !liveLog.Cleanup.Enabled {
				return 0, nil
			}
			retentionDays := liveLog.Cleanup.RetentionDays
			if retentionDays < 1 {
				retentionDays = defaultLogRetentionDay
			}
			return writer.CleanupExpiredLogs(liveLog.Dir, retentionDays, infra.Logger)
		},
	}); err != nil {
		infra.Logger.Errorf("注册日志清理任务失败: %v", err)
		return
	}
	infra.Logger.Infof("已注册日志清理任务: 每日执行时间=%02d:%02d:%02d（开关与保留天数按运行时配置现读）",
		hour, minute, second)
}
