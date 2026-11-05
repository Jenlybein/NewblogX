//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/chat_msg_enum"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 真库版的并账 tick 用例（#279 票面要求「真库用例验周期配置化、失败隔离」）。
//
// 与 `cron_reconcile_tick_test.go`（sqlite）的分工：
//   - sqlite 那份守"周期来自配置 / 一个任务体 / 失败隔离"的**形状**，跑得快；
//   - 这份守真 MySQL 上同样成立：覆盖写的 affected rows、真外键、真索引。
//
// 装配与调度接缝复用同包的记录器（`wireRecordingCron`），断言只经公开行为：
// 注册了什么周期的任务、跑一次之后库里的派生列变成什么。

// newCronMySQLInfra 造一份跑在真 MySQL 上的最小 infra。
func newCronMySQLInfra(t *testing.T, intervalMinutes int, groups ...schema.Group) *Infra {
	t.Helper()

	db := mysqlcontainer.DB(t, schema.Models(groups...)...)

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

// seedRealMySQLReconcileData 造"列已漂移"的文章与私信各一条（真外键约束下先建用户）。
func seedRealMySQLReconcileData(t *testing.T, infra *Infra) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, id := range []uint64{100, 200, 301, 302} {
		schema.InsertUser(t, infra.DB, schema.UserSpec{
			ID: id, Username: "reconcile_" + ctype.NewID(id).String(), Nickname: "并账用户", CreatedAt: at, UpdatedAt: at,
		})
	}

	// 文章：列上 9（漂移），关系行 2 条。
	schema.InsertArticle(t, infra.DB, schema.ArticleSpec{
		ID:            1001,
		Title:         "真库并账文章",
		AuthorID:      100,
		DiggCount:     9,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     at,
		UpdatedAt:     at,
	})
	schema.InsertArticleDigg(t, infra.DB, schema.ArticleDiggSpec{ID: 6001, ArticleID: 1001, UserID: 301})
	schema.InsertArticleDigg(t, infra.DB, schema.ArticleDiggSpec{ID: 6002, ArticleID: 1001, UserID: 302})
	if err := infra.Redis.HIncrBy(context.Background(), string(redis_article.ArticleCacheDigg), "1001", 1).Err(); err != nil {
		t.Fatalf("写入点赞增量失败: %v", err)
	}

	// 私信会话：列上 9（漂移），消息行真值是 1 条未读。
	if err := infra.DB.Create(&schema.ChatSessionRow{
		BaseRow:     schema.BaseRow{ID: ctype.NewID(1), CreatedAt: at, UpdatedAt: at},
		SessionID:   "chat:100:200",
		UserID:      ctype.NewID(100),
		ReceiverID:  ctype.NewID(200),
		UnreadCount: 9,
	}).Error; err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}
	if err := infra.DB.Create(&schema.ChatMsgRow{
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

// TestReconcileTickOnRealMySQL 真库：周期来自配置，一次任务体同时做两个并账。
func TestReconcileTickOnRealMySQL(t *testing.T) {
	infra := newCronMySQLInfra(t, 7, schema.UserCore, schema.ArticleCore, schema.Chat)
	seedRealMySQLReconcileData(t, infra)

	job := wireRecordingCron(t, infra).reconcileJob(t)
	if job.interval != 7*time.Minute {
		t.Fatalf("并账周期应取运行期配置的 7 分钟，实际 %s", job.interval)
	}

	affected, err := job.job.Run(context.Background())
	if err != nil {
		t.Fatalf("真库并账不应失败: %v", err)
	}
	if affected == 0 {
		t.Fatal("一次并账至少应处理一条（文章计数或私信未读）")
	}
	if got := readDiggCount(t, infra.DB); got != 2 {
		t.Fatalf("真库上文章点赞数应按关系行重算为 2，实际 %d", got)
	}
	if got := readUnread(t, infra.DB); got != 1 {
		t.Fatalf("真库上私信未读应按消息行重算为 1，实际 %d", got)
	}
}

// TestReconcileTickIsolatesChatFailureOnRealMySQL 真库：私信并账失败不挡住文章计数。
func TestReconcileTickIsolatesChatFailureOnRealMySQL(t *testing.T) {
	// 只建文章侧的表：私信并账会真的因为"表不存在"失败。
	infra := newCronMySQLInfra(t, 30, schema.UserCore, schema.ArticleCore)
	seedArticleCounterOnly(t, infra)

	_, err := wireRecordingCron(t, infra).reconcileJob(t).job.Run(context.Background())
	if err == nil {
		t.Fatal("私信并账失败必须返回错误（否则运维看不到）")
	}
	if got := readDiggCount(t, infra.DB); got != 2 {
		t.Fatalf("私信并账失败不应挡住文章计数：点赞数应为 2，实际 %d", got)
	}
}

// seedArticleCounterOnly 只造文章侧数据（不建私信表时的隔离用例）。
func seedArticleCounterOnly(t *testing.T, infra *Infra) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{100, 301, 302} {
		schema.InsertUser(t, infra.DB, schema.UserSpec{
			ID: id, Username: "reconcile_only_" + ctype.NewID(id).String(), Nickname: "并账用户", CreatedAt: at, UpdatedAt: at,
		})
	}
	schema.InsertArticle(t, infra.DB, schema.ArticleSpec{
		ID:            1001,
		Title:         "真库并账文章",
		AuthorID:      100,
		DiggCount:     9,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     at,
		UpdatedAt:     at,
	})
	schema.InsertArticleDigg(t, infra.DB, schema.ArticleDiggSpec{ID: 6001, ArticleID: 1001, UserID: 301})
	schema.InsertArticleDigg(t, infra.DB, schema.ArticleDiggSpec{ID: 6002, ArticleID: 1001, UserID: 302})
	if err := infra.Redis.HIncrBy(context.Background(), string(redis_article.ArticleCacheDigg), "1001", 1).Err(); err != nil {
		t.Fatalf("写入点赞增量失败: %v", err)
	}
}
