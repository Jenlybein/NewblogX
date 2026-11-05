package app

import (
	"context"
	"strconv"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/chat_msg_enum"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/scheduler"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"
)

// 本文件守 #279 的「一处配置、一个调度任务、失败彼此隔离」三条口径。
//
// 为什么要替换掉原来的源码文本守卫（`cron_chat_unread_wiring_test.go`）：
// 它断言 `strings.Count(text, "sch.Every(counterSyncInterval")`，只能证明
// "这行字还在"，既看不见周期是否真的来自配置，也看不见两个并账是否真的
// 跑在同一个任务体里。现在改成在调度接缝上观察行为：注册了什么任务、
// 周期是多少、跑一次任务体会发生什么。
//
// 装配层没有 HTTP 面，所以这里的接缝是调度注册接口（schedulerRegistrar）——
// 它仍然是"对外可见的行为"：任务名、周期、执行效果。

// recordedJob 是一次注册记录。
type recordedJob struct {
	interval time.Duration
	job      scheduler.Job
}

// recordingScheduler 记录 WireCron 注册了什么，并允许用例直接驱动任务体。
type recordingScheduler struct {
	every   []recordedJob
	daily   []recordedJob
	started bool
}

func (s *recordingScheduler) Every(d time.Duration, job scheduler.Job) error {
	s.every = append(s.every, recordedJob{interval: d, job: job})
	return nil
}

func (s *recordingScheduler) Daily(hour, minute, second int, job scheduler.Job) error {
	s.daily = append(s.daily, recordedJob{job: job})
	return nil
}

func (s *recordingScheduler) Start()         { s.started = true }
func (s *recordingScheduler) Shutdown() error { return nil }

// reconcileJob 取出并账任务；没有就说明装配漏了这条任务。
func (s *recordingScheduler) reconcileJob(t *testing.T) recordedJob {
	t.Helper()
	for _, e := range s.every {
		if e.job.Name == "同步文章计数" {
			return e
		}
	}
	t.Fatalf("必须注册「同步文章计数」并账任务，实际注册了 %d 个 Every 任务", len(s.every))
	return recordedJob{}
}

// wireRecordingCron 用记录器装配定时任务，并返回记录器。
func wireRecordingCron(t *testing.T, infra *Infra) *recordingScheduler {
	t.Helper()
	sched := &recordingScheduler{}
	if err := wireCron(infra, nil, func() (taskScheduler, error) { return sched, nil }); err != nil {
		t.Fatalf("装配定时任务失败: %v", err)
	}
	if !sched.started {
		t.Fatal("装配完成后调度器必须已启动")
	}
	return sched
}

// newCronTestInfra 造一份最小 infra：sqlite + miniredis + 运行期配置。
//
// intervalMinutes 直接写进运行期配置的默认值，等价于「管理员把
// site.reconcile.interval_minutes 改成 N」，不是给代码塞常量。
func newCronTestInfra(t *testing.T, intervalMinutes int, groups ...schema.Group) *Infra {
	t.Helper()

	db := testkit.NewSQLite(t, schema.Models(groups...)...)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	runtimeSite := config.NewRuntimeConfigService(conf.RuntimeDefaults{
		Site: conf.Site{Reconcile: siteconf.Reconcile{IntervalMinutes: intervalMinutes}},
	}, logger, db)

	return &Infra{
		Config:      &conf.Config{},
		Logger:      logger,
		DB:          db,
		Redis:       rdb,
		RuntimeSite: runtimeSite,
	}
}

// seedArticleCounterDrift 造一篇文章：列上 9（漂移）、关系行 2 条、Redis 增量 1。
//
// 并账做的是「按关系行重算覆盖」，所以跑完必须变成 2（不是 9+1），
// 这就是"任务体真的执行了"的可观测证据。
func seedArticleCounterDrift(t *testing.T, db *gorm.DB, rdb *redis.Client) {
	t.Helper()
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            1001,
		Title:         "并账文章",
		AuthorID:      100,
		DiggCount:     9,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
	})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6001, ArticleID: 1001, UserID: 301})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6002, ArticleID: 1001, UserID: 302})

	if err := rdb.HIncrBy(context.Background(), string(redis_article.ArticleCacheDigg), "1001", 1).Err(); err != nil {
		t.Fatalf("写入点赞增量失败: %v", err)
	}
}

// readDiggCount 回读文章行的点赞列（并账后的持久值）。
func readDiggCount(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var row schema.ArticleRow
	if err := db.First(&row, "id = ?", 1001).Error; err != nil {
		t.Fatalf("回查文章失败: %v", err)
	}
	return row.DiggCount
}

// seedChatUnreadDrift 造一条会话：列上 9（漂移）、真值是 1 条未读。
func seedChatUnreadDrift(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{ID: 100, Username: "chat_a", Nickname: "甲"})
	schema.InsertUser(t, db, schema.UserSpec{ID: 200, Username: "chat_b", Nickname: "乙"})

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := db.Create(&schema.ChatSessionRow{
		BaseRow:     schema.BaseRow{ID: ctype.NewID(1), CreatedAt: at, UpdatedAt: at},
		SessionID:   "chat:100:200",
		UserID:      ctype.NewID(100),
		ReceiverID:  ctype.NewID(200),
		UnreadCount: 9,
	}).Error; err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}
	if err := db.Create(&schema.ChatMsgRow{
		BaseRow:    schema.BaseRow{ID: ctype.NewID(9001), CreatedAt: at, UpdatedAt: at},
		SenderID:   ctype.NewID(200),
		ReceiverID: ctype.NewID(100),
		SessionID:  "chat:100:200",
		Content:    "你好",
		SendTime:   at,
		MsgStatus:  chat_msg_enum.MsgStatusSend,
		MsgType:    chat_msg_enum.MsgTypeText,
	}).Error; err != nil {
		t.Fatalf("插入消息失败: %v", err)
	}
}

// readUnread 回读会话行的未读列（并账后的派生值）。
func readUnread(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var row schema.ChatSessionRow
	if err := db.First(&row, "id = ?", 1).Error; err != nil {
		t.Fatalf("回查会话失败: %v", err)
	}
	return row.UnreadCount
}

// TestReconcilePeriodComesFromRuntimeConfig 周期来自运行期配置，且只有一个并账任务。
func TestReconcilePeriodComesFromRuntimeConfig(t *testing.T) {
	infra := newCronTestInfra(t, 5, schema.UserCore, schema.ArticleCore, schema.Chat)
	sched := wireRecordingCron(t, infra)

	job := sched.reconcileJob(t)
	if job.interval != 5*time.Minute {
		t.Fatalf("并账周期应取运行期配置的 5 分钟，实际 %s", job.interval)
	}

	// 「一个调度任务」不是靠数源码行数：所有计数并账任务必须共用同一个周期，
	// 谁另外注册一个别的节奏，这里立刻看得见。
	for _, e := range sched.every {
		if e.interval != 5*time.Minute {
			t.Fatalf("任务 %q 的周期是 %s，应与其他并账任务同为运行期配置的 5 分钟", e.job.Name, e.interval)
		}
	}
}

// TestReconcileTaskRunsBothReconcilesInOnePass 一次任务体同时刷文章计数与私信未读。
func TestReconcileTaskRunsBothReconcilesInOnePass(t *testing.T) {
	infra := newCronTestInfra(t, 30, schema.UserCore, schema.ArticleCore, schema.Chat)
	seedArticleCounterDrift(t, infra.DB, infra.Redis)
	seedChatUnreadDrift(t, infra.DB)

	affected, err := wireRecordingCron(t, infra).reconcileJob(t).job.Run(context.Background())
	if err != nil {
		t.Fatalf("并账不应失败: %v", err)
	}
	if affected == 0 {
		t.Fatal("一次并账应至少处理一条（文章计数或私信未读）")
	}

	if got := readDiggCount(t, infra.DB); got != 2 {
		t.Fatalf("文章点赞数应按关系行重算为 2，实际 %d", got)
	}
	if got := readUnread(t, infra.DB); got != 1 {
		t.Fatalf("私信未读应按消息行重算为 1，实际 %d", got)
	}
}

// TestReconcileTaskIsolatesChatFailure 私信并账失败不挡住文章计数。
func TestReconcileTaskIsolatesChatFailure(t *testing.T) {
	// 只建文章侧的表：私信并账必然因为表不存在而失败。
	infra := newCronTestInfra(t, 30, schema.UserCore, schema.ArticleCore)
	seedArticleCounterDrift(t, infra.DB, infra.Redis)

	_, err := wireRecordingCron(t, infra).reconcileJob(t).job.Run(context.Background())
	if err == nil {
		t.Fatal("私信并账失败必须返回错误（否则运维看不到）")
	}
	if got := readDiggCount(t, infra.DB); got != 2 {
		t.Fatalf("私信并账失败不应挡住文章计数：点赞数应为 2，实际 %d", got)
	}
}

// TestReconcileTaskIsolatesArticleFailure 文章计数并账失败不挡住私信未读。
func TestReconcileTaskIsolatesArticleFailure(t *testing.T) {
	// 只建私信侧的表：文章计数并账会逐指标失败。
	infra := newCronTestInfra(t, 30, schema.UserCore, schema.Chat)
	seedChatUnreadDrift(t, infra.DB)
	addDiggDelta(t, infra.Redis)

	if _, err := wireRecordingCron(t, infra).reconcileJob(t).job.Run(context.Background()); err != nil {
		t.Fatalf("文章侧失败不应让整个并账失败（历史口径：逐指标记录后继续）: %v", err)
	}
	if got := readUnread(t, infra.DB); got != 1 {
		t.Fatalf("文章计数并账失败不应挡住私信未读：应为 1，实际 %d", got)
	}
}

// addDiggDelta 给文章计数放一条增量，逼并账真的去写库。
func addDiggDelta(t *testing.T, rdb *redis.Client) {
	t.Helper()
	field := strconv.FormatUint(1001, 10)
	if err := rdb.HIncrBy(context.Background(), string(redis_article.ArticleCacheDigg), field, 1).Err(); err != nil {
		t.Fatalf("写入点赞增量失败: %v", err)
	}
}
